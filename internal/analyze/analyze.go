package analyze

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"luanti-version-scan/internal/db"
)

var (
	modConfNameRe          = regexp.MustCompile(`^\s*name\s*=\s*(.+)$`)
	modConfVersionRe       = regexp.MustCompile(`^\s*minetest_version\s*=\s*(.+)$`)
	modConfFormspecVersion = regexp.MustCompile(`^\s*formspec_version\s*=\s*(\d+)$`)
	ifRe                   = regexp.MustCompile(`\bif\s+(.*?)\s+then\b`)
	elseifRe               = regexp.MustCompile(`\belseif\s+(.*?)\s+then\b`)
	elseRe                 = regexp.MustCompile(`\belse\b`)
	endRe                  = regexp.MustCompile(`\bend\b`)
	untilRe                = regexp.MustCompile(`\buntil\b`)
	forRe                  = regexp.MustCompile(`\bfor\s+.*?\s+do\b`)
	whileRe                = regexp.MustCompile(`\bwhile\s+.*?\s+do\b`)
	repeatRe               = regexp.MustCompile(`\brepeat\b`)
	funcRe                 = regexp.MustCompile(`\bfunction\b`)
	minetestFeaturesRe     = regexp.MustCompile(`(?:minetest|core)\.features\.(\w+)`)
	minetestAPICallRe      = regexp.MustCompile(`(?:minetest|core)\.(\w+)\s*\(`)
	methodCallRe           = regexp.MustCompile(`(\w+):(\w+)\s*\(`)
	versionCheckRe         = regexp.MustCompile(`(?:minetest|core)\.get_version\s*\(`)
	minetestAPIBareRe      = regexp.MustCompile(`(?:minetest|core)\.(\w+)`)
	methodBareRe           = regexp.MustCompile(`(\w+):(\w+)`)

	luaBuiltins = map[string]bool{
		"sub": true, "gsub": true, "match": true, "find": true,
		"upper": true, "lower": true, "len": true, "rep": true,
		"reverse": true, "char": true, "byte": true, "format": true,
		"gmatch": true, "dump": true, "insert": true, "remove": true,
		"sort": true, "concat": true, "pairs": true, "ipairs": true,
		"next": true, "type": true, "tostring": true, "tonumber": true,
		"split": true,
		"close": true, "read": true, "write": true, "seek": true,
		"flush": true, "lines": true,
	}
)

type ParamUse struct {
	Name       string
	Version    string
	MaxVersion string
	Known      bool
	Detected   bool
}

type FeatureUse struct {
	Name          string
	File          string
	Line          int
	IsGuard       bool
	Guarded       bool
	GuardName     string
	Version       string
	MaxVersion    string
	Known         bool
	UseCorePrefix bool
	Params        []ParamUse
}

type FileFormatUse struct {
	Ext     string
	Version string
}

type Analysis struct {
	ModName         string
	MinetestVersion string
	FormspecVersion int
	Depends         []string
	Uses            []FeatureUse
	FeaturesChecked []string
	FileFormats     []FileFormatUse
}

func WalkAndAnalyze(root string, database *db.DB) (*Analysis, error) {
	a := &Analysis{}
	fileExts := make(map[string]bool)
	modVarTypes := extractModVarTypes(root)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(root, path)

		ext := strings.ToLower(filepath.Ext(path))
		if ext != "" {
			fileExts[ext] = true
		}

		switch filepath.Base(path) {
		case "mod.conf":
			parseModConf(path, a)
		case "depends.txt":
			parseDependsTxt(path, a)
		}

		if strings.HasSuffix(path, ".lua") {
			analyzeFile(path, rel, a, database, modVarTypes)
		}
		return nil
	})

	for ext := range fileExts {
		entry, ok := database.LookupFileFormat(ext)
		if ok {
			a.FileFormats = append(a.FileFormats, FileFormatUse{
				Ext:     ext,
				Version: entry.Version,
			})
		}
	}

	// Deduplicate feature checks
	seen := make(map[string]bool)
	for _, u := range a.Uses {
		if u.IsGuard && !seen[u.Name] {
			a.FeaturesChecked = append(a.FeaturesChecked, u.Name)
			seen[u.Name] = true
		}
	}

	return a, nil
}

func parseModConf(path string, a *Analysis) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if match := modConfNameRe.FindStringSubmatch(line); match != nil {
			a.ModName = strings.TrimSpace(match[1])
		}
		if match := modConfVersionRe.FindStringSubmatch(line); match != nil {
			a.MinetestVersion = strings.TrimSpace(match[1])
		}
		if match := modConfFormspecVersion.FindStringSubmatch(line); match != nil {
			a.FormspecVersion, _ = strconv.Atoi(match[1])
		}
	}
}

func parseDependsTxt(path string, a *Analysis) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			mod := strings.TrimSuffix(line, "?")
			a.Depends = append(a.Depends, mod)
		}
	}
}

// findGuardName scans a condition for bare function/method references (no parens),
// indicating an existence/feature check. Returns the first such name found,
// or "" if none. Handles: core.A, core.A and core.B, obj:method, not core.A, etc.
func findGuardName(cond string) string {
	for _, m := range minetestAPIBareRe.FindAllStringSubmatchIndex(cond, -1) {
		end := m[1]
		if end < len(cond) && (cond[end] == '(' || cond[end] == '[' || cond[end] == '.' || cond[end] == ':') {
			continue
		}
		return cond[m[2]:m[3]]
	}
	for _, m := range methodBareRe.FindAllStringSubmatchIndex(cond, -1) {
		obj := cond[m[2]:m[3]]
		method := cond[m[4]:m[5]]
		if obj == "minetest" || obj == "core" ||
			obj == "self" || obj == "true" || obj == "false" || obj == "nil" {
			continue
		}
		if luaBuiltins[method] {
			continue
		}
		end := m[1]
		if end < len(cond) && (cond[end] == '(' || cond[end] == '[' || cond[end] == '.' || cond[end] == ':') {
			continue
		}
		return obj + ":" + method
	}
	return ""
}

func analyzeFile(path, rel string, a *Analysis, database *db.DB, modVarTypes map[string]string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	src := string(data)
	clean := stripCommentsAndStrings(src)
	lines := strings.Split(clean, "\n")

	// Track line offsets in the full clean source for cross-line arg extraction
	lineOffsets := make([]int, len(lines))
	offset := 0
	for i, line := range lines {
		lineOffsets[i] = offset
		offset += len(line) + 1 // +1 for newline
	}

	type guardInfo struct {
		name      string
		isFeature bool // true = minetest.features.X, false = negation or version
	}

	type block struct {
		kind  string // "if", "elseif", "else", "for", "while", "repeat", "function", "do"
		guard *guardInfo
	}

	var stack []block

	currentGuard := func() *guardInfo {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].guard != nil {
				return stack[i].guard
			}
		}
		return nil
	}

	isGuarded := func() bool {
		return currentGuard() != nil
	}

	guardName := func() string {
		g := currentGuard()
		if g == nil {
			return ""
		}
		return g.name
	}

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		origLine := line
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// --- Detect feature checks (anywhere on the line, before block tracking) ---
		if fm := minetestFeaturesRe.FindStringSubmatch(line); fm != nil {
			featureName := fm[1]
			entry, known := database.LookupFeature(featureName)
			ver := ""
			var maxVer string
			if known {
				ver = entry.Version
				maxVer = entry.MaxVersion
			}
			a.Uses = append(a.Uses, FeatureUse{
				Name:       "core.features." + featureName,
				File:       rel,
				Line:       lineNum,
				IsGuard:    true,
				Guarded:    isGuarded(),
				GuardName:  guardName(),
				Version:    ver,
				MaxVersion: maxVer,
				Known:      known,
			})
		}

		// --- if condition then ---
		if matches := ifRe.FindStringSubmatch(line); matches != nil {
			cond := matches[1]
			var g *guardInfo

			if fm := minetestFeaturesRe.FindStringSubmatch(cond); fm != nil {
				g = &guardInfo{name: fm[1], isFeature: true}
			} else if gn := findGuardName(cond); gn != "" {
				g = &guardInfo{name: gn, isFeature: true}
			} else if versionCheckRe.MatchString(cond) {
				g = &guardInfo{name: "minetest.get_version()", isFeature: false}
			}

			stack = append(stack, block{kind: "if", guard: g})
			continue
		}

		// --- elseif condition then ---
		if matches := elseifRe.FindStringSubmatch(line); matches != nil {
			cond := matches[1]
			if len(stack) > 0 && (stack[len(stack)-1].kind == "if" || stack[len(stack)-1].kind == "elseif") {
				stack = stack[:len(stack)-1]
			}

			var g *guardInfo
			if fm := minetestFeaturesRe.FindStringSubmatch(cond); fm != nil {
				g = &guardInfo{name: fm[1], isFeature: true}
			} else if gn := findGuardName(cond); gn != "" {
				g = &guardInfo{name: gn, isFeature: true}
			} else if versionCheckRe.MatchString(cond) {
				g = &guardInfo{name: "minetest.get_version()", isFeature: false}
			}

			stack = append(stack, block{kind: "elseif", guard: g})
			continue
		}

		// --- else ---
		if elseRe.MatchString(line) {
			word := line
			if idx := strings.Index(line, " "); idx != -1 {
				word = line[:idx]
			}
			if word == "else" && len(line) > 4 && !strings.HasPrefix(line, "elseif") {
				if len(stack) > 0 && (stack[len(stack)-1].kind == "if" || stack[len(stack)-1].kind == "elseif") {
					parent := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					var g *guardInfo
					if parent.guard != nil {
						g = &guardInfo{name: "not " + parent.guard.name, isFeature: false}
					}
					stack = append(stack, block{kind: "else", guard: g})
				}
				continue
			}
		}

		// --- end ---
		if endRe.MatchString(line) {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}

		// --- for ... do ---
		if forRe.MatchString(line) {
			stack = append(stack, block{kind: "for"})
			continue
		}

		// --- while ... do ---
		if whileRe.MatchString(line) {
			stack = append(stack, block{kind: "while"})
			continue
		}

		// --- repeat ---
		if repeatRe.MatchString(line) {
			stack = append(stack, block{kind: "repeat"})
			continue
		}

		// --- until ---
		if untilRe.MatchString(line) {
			if len(stack) > 0 && stack[len(stack)-1].kind == "repeat" {
				stack = stack[:len(stack)-1]
			}
			continue
		}

		// --- function ---
		if funcRe.MatchString(line) {
			stack = append(stack, block{kind: "function"})
			continue
		}

		// --- do (standalone) ---
		if strings.HasPrefix(line, "do") && (len(line) == 2 || line[2] == ' ' || line[2] == '\t') {
			// Make sure it's not for ... do or while ... do
			if !forRe.MatchString(line) && !whileRe.MatchString(line) {
				stack = append(stack, block{kind: "do"})
				continue
			}
		}

		// --- minetest.XXX(...) calls ---
		if apiMatches := minetestAPICallRe.FindAllStringSubmatchIndex(origLine, -1); apiMatches != nil {
			for _, m := range apiMatches {
				funcName := origLine[m[2]:m[3]]

				// skip minetest.features (it's handled above)
				if funcName == "features" {
					continue
				}

				// Detect whether call used core. or minetest. prefix
				prefix := origLine[m[0] : m[2]-1]
				useCore := prefix == "core"

				lookupName := "minetest." + funcName
				displayName := "core." + funcName

				entry, known := database.LookupAPI(lookupName)
				ver := ""
				var maxVer string
				var dbParams []ParamUse
				if known {
					ver = entry.Version
					maxVer = entry.MaxVersion
					for pn, pe := range entry.Params {
						dbParams = append(dbParams, ParamUse{
							Name:       pn,
							Version:    pe.Version,
							MaxVersion: pe.MaxVersion,
							Known:      true,
						})
					}
				}

				usage := FeatureUse{
					Name:          displayName,
					File:          rel,
					Line:          lineNum,
					IsGuard:       false,
					Guarded:       isGuarded(),
					GuardName:     guardName(),
					Version:       ver,
					MaxVersion:    maxVer,
					Known:         known,
					UseCorePrefix: useCore,
				}

				// Try to detect used params from source
				if known && len(dbParams) > 0 {
					parenPos := lineOffsets[lineIdx] + m[1]
					usedKeys := extractTableKeysFromCall(clean, parenPos)
					for i, dp := range dbParams {
						for _, k := range usedKeys {
							if k == dp.Name {
								dbParams[i].Detected = true
								break
							}
						}
					}
					usage.Params = dbParams
				}

				a.Uses = append(a.Uses, usage)
			}
		}

		// --- obj:method(...) calls ---
		if methodMatches := methodCallRe.FindAllStringSubmatchIndex(origLine, -1); methodMatches != nil {
			for _, m := range methodMatches {
				obj := origLine[m[2]:m[3]]
				method := origLine[m[4]:m[5]]

				if obj == "minetest" || obj == "core" {
					continue
				}
				if obj == "self" || obj == "true" || obj == "false" || obj == "nil" {
					continue
				}

			if luaBuiltins[method] {
					continue
				}

				// Map variable names to object type prefixes
				objPrefix := objPrefixFor(obj)
				var ver string
				var maxVer string
				var known bool
				var dbParams []ParamUse
				if objPrefix != "" {
					entry, ok := database.LookupAPI(objPrefix + method)
					if ok {
						ver = entry.Version
						maxVer = entry.MaxVersion
						known = true
						for pn, pe := range entry.Params {
							dbParams = append(dbParams, ParamUse{
								Name:       pn,
								Version:    pe.Version,
								MaxVersion: pe.MaxVersion,
								Known:      true,
							})
						}
					}
				}
				// Fallback: try all known prefixes
				if !known {
					for _, prefix := range []string{"object:", "voxelmanip:", "voxelarea:", "itemstack:", "storage:", "file:", "inventory:", "settings:"} {
						entry, ok := database.LookupAPI(prefix + method)
						if ok {
							ver = entry.Version
							maxVer = entry.MaxVersion
							known = true
							for pn, pe := range entry.Params {
								dbParams = append(dbParams, ParamUse{
									Name:       pn,
									Version:    pe.Version,
									MaxVersion: pe.MaxVersion,
									Known:      true,
								})
							}
							break
						}
					}
				}

				// Filter out mod-provided API calls
				if !known {
					if objPrefixFor(obj) == "" {
						continue
					}
					if t, ok := modVarTypes[obj]; ok && (t == "mod" || t == "param") {
						continue
					}
				}

				// Try to detect used params from source
				if known && len(dbParams) > 0 {
					parenPos := lineOffsets[lineIdx] + m[1]
					usedKeys := extractTableKeysFromCall(clean, parenPos)
					for i, dp := range dbParams {
						for _, k := range usedKeys {
							if k == dp.Name {
								dbParams[i].Detected = true
								break
							}
						}
					}
				}

				usage := FeatureUse{
					Name:       obj + ":" + method,
					File:       rel,
					Line:       lineNum,
					IsGuard:    false,
					Guarded:    isGuarded(),
					GuardName:  guardName(),
					Version:    ver,
					MaxVersion: maxVer,
					Known:      known,
					Params:     dbParams,
				}
				a.Uses = append(a.Uses, usage)
			}
		}
	}
}

// extractModVarTypes walks all .lua files in root to identify variable names
// defined within the mod. Returns a map of var name → type:
// "mod"  = defined as a local table/function or has function definitions (mod-provided API)
// "param" = function parameter (potentially mod-provided when method is unknown)
func extractModVarTypes(root string) map[string]string {
	varTypes := make(map[string]string)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".lua") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		clean := stripCommentsAndStrings(string(data))

		funcRe := regexp.MustCompile(`function\s+(\w+)[\.:]`)
		for _, m := range funcRe.FindAllStringSubmatch(clean, -1) {
			varTypes[m[1]] = "mod"
		}

		tableRe := regexp.MustCompile(`local\s+(\w+)\s*=\s*\{`)
		for _, m := range tableRe.FindAllStringSubmatch(clean, -1) {
			if _, exists := varTypes[m[1]]; !exists {
				varTypes[m[1]] = "mod"
			}
		}

		funcAssignRe := regexp.MustCompile(`local\s+(\w+)\s*=\s*function\s*\(`)
		for _, m := range funcAssignRe.FindAllStringSubmatch(clean, -1) {
			varTypes[m[1]] = "mod"
		}

		moduleRe := regexp.MustCompile(`local\s+(\w+)\s*=\s*(?:require|dofile|loadfile)\s*\(`)
		for _, m := range moduleRe.FindAllStringSubmatch(clean, -1) {
			varTypes[m[1]] = "mod"
		}

		// Named function parameters: function name(a, b, c)
		paramRe := regexp.MustCompile(`function\s+\w+\s*\(`)
		for _, m := range paramRe.FindAllStringSubmatchIndex(clean, -1) {
			parenPos := m[1] - 1
			endParen := findMatchingBrace(clean, parenPos+1, "()")
			if endParen != -1 {
				paramsStr := clean[parenPos+1 : endParen]
				params := strings.Split(paramsStr, ",")
				for _, p := range params {
					p = strings.TrimSpace(p)
					if p != "" && !strings.HasPrefix(p, "...") {
						if _, exists := varTypes[p]; !exists {
							varTypes[p] = "param"
						}
					}
				}
			}
		}

		// Anonymous function parameters: function(a, b, c)
		anonRe := regexp.MustCompile(`function\s*\(`)
		for _, m := range anonRe.FindAllStringSubmatchIndex(clean, -1) {
			parenPos := m[1] - 1
			endParen := findMatchingBrace(clean, parenPos+1, "()")
			if endParen != -1 {
				paramsStr := clean[parenPos+1 : endParen]
				params := strings.Split(paramsStr, ",")
				for _, p := range params {
					p = strings.TrimSpace(p)
					if p != "" && !strings.HasPrefix(p, "...") {
						if _, exists := varTypes[p]; !exists {
							varTypes[p] = "param"
						}
					}
				}
			}
		}

		return nil
	})

	return varTypes
}

// findMatchingBrace finds the matching closing brace/bracket/paren
// starting from pos (which should be just past the opening delimiter).
// delims are pairs of open/close chars, e.g. "{},()".
func findMatchingBrace(s string, pos int, delims string) int {
	depth := 0
	for pos < len(s) {
		ch := s[pos]
		for i := 0; i < len(delims); i += 2 {
			if ch == delims[i] {
				depth++
				break
			}
			if ch == delims[i+1] {
				if depth == 0 {
					return pos
				}
				depth--
				break
			}
		}
		pos++
	}
	return -1
}

// extractTableKeysFromCall scans from just after the opening '(' of a call,
// finds the matching ')', and extracts top-level table keys from
// any table constructor arguments { key = value, ... }.
func extractTableKeysFromCall(clean string, afterParen int) []string {
	if afterParen >= len(clean) {
		return nil
	}

	// Find matching ')'
	endPos := findMatchingBrace(clean, afterParen, "(){}")
	if endPos == -1 {
		return nil
	}

	args := clean[afterParen:endPos]

	// Extract top-level table constructors from args
	var keys []string
	for i := 0; i < len(args); i++ {
		if args[i] == '{' {
			tableEnd := findMatchingBrace(args, i+1, "{}")
			if tableEnd == -1 {
				continue
			}
			tableContent := args[i+1 : tableEnd]
			// Find key = value patterns at the top level of the table
			keyRe := regexp.MustCompile(`(\w+)\s*=`)
			for _, km := range keyRe.FindAllStringSubmatch(tableContent, -1) {
				keys = append(keys, km[1])
			}
			i = tableEnd
		}
	}
	return keys
}

func objPrefixFor(obj string) string {
	// Common Luanti object variable names mapped to DB prefixes
	switch obj {
	case "player", "players", "playername", "clicker", "obj", "object", "entity", "placer", "user", "actor":
		return "object:"
	case "VoxelArea", "voxelarea":
		return "voxelarea:"
	case "vm", "voxel", "voxelmanip", "manip":
		return "voxelmanip:"
	case "item", "items", "stack", "itemstack", "wielded", "new_stack":
		return "itemstack:"
	case "storage", "mod_storage", "meta", "metadata", "data", "block_meta", "node_meta":
		return "storage:"
	case "file", "f", "fh", "handle", "filehandle":
		return "file:"
	case "inv", "inventory", "invref", "list", "player_inv", "main_inv":
		return "inventory:"
	case "settings", "core_settings":
		return "settings:"
	}
	// Check for substring hints before falling back to first-char heuristic
	lower := strings.ToLower(obj)
	if strings.Contains(lower, "stack") || strings.Contains(lower, "item") {
		return "itemstack:"
	}
	if strings.Contains(lower, "file") {
		return "file:"
	}
	if strings.Contains(lower, "inv") {
		return "inventory:"
	}
	if strings.Contains(lower, "voxelarea") {
		return "voxelarea:"
	}
	if strings.Contains(lower, "voxel") || strings.Contains(lower, "manip") {
		return "voxelmanip:"
	}
	if strings.Contains(lower, "storage") || strings.Contains(lower, "meta") || strings.Contains(lower, "data") {
		return "storage:"
	}
	if strings.Contains(lower, "settings") {
		return "settings:"
	}
	if strings.Contains(lower, "player") || strings.Contains(lower, "entity") || strings.Contains(lower, "object") {
		return "object:"
	}

	return ""
}

func stripCommentsAndStrings(src string) string {
	var result strings.Builder
	result.Grow(len(src))

	i := 0
	for i < len(src) {
		// --- Long comment --[[ ... ]] or --[=[...]=] ---
		if i+3 < len(src) && src[i] == '-' && src[i+1] == '-' && src[i+2] == '[' && src[i+3] == '[' {
			result.WriteByte(' ')
			i += 4
			// Find ]] to close (simple case, no nesting)
			for i+1 < len(src) {
				if src[i] == ']' && src[i+1] == ']' {
					i += 2
					break
				}
				if src[i] == '\n' {
					result.WriteByte('\n')
				} else {
					result.WriteByte(' ')
				}
				i++
			}
			continue
		}

		// --- Long comment --[=[...]=] (with = signs) ---
		if i+2 < len(src) && src[i] == '-' && src[i+1] == '-' && src[i+2] == '[' {
			result.WriteByte(' ')
			i += 3
			eqCount := 0
			for i < len(src) && src[i] == '=' {
				eqCount++
				i++
			}
			if i < len(src) && src[i] == '[' {
				i++
				closeStr := "]" + strings.Repeat("=", eqCount) + "]"
				for i+len(closeStr)-1 < len(src) {
					if src[i:i+len(closeStr)] == closeStr {
						i += len(closeStr)
						break
					}
					if src[i] == '\n' {
						result.WriteByte('\n')
					} else {
						result.WriteByte(' ')
					}
					i++
				}
			}
			continue
		}

		// --- Line comment -- ---
		if i+1 < len(src) && src[i] == '-' && src[i+1] == '-' {
			result.WriteByte(' ')
			i += 2
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}

		// --- Single-quoted string ---
		if src[i] == '\'' {
			result.WriteByte(' ')
			i++
			for i < len(src) {
				if src[i] == '\\' {
					i += 2
					continue
				}
				if src[i] == '\'' {
					i++
					break
				}
				if src[i] == '\n' {
					break
				}
				result.WriteByte(' ')
				i++
			}
			continue
		}

		// --- Double-quoted string ---
		if src[i] == '"' {
			result.WriteByte(' ')
			i++
			for i < len(src) {
				if src[i] == '\\' {
					i += 2
					continue
				}
				if src[i] == '"' {
					i++
					break
				}
				if src[i] == '\n' {
					break
				}
				result.WriteByte(' ')
				i++
			}
			continue
		}

		// --- Long string [[ ... ]] ---
		if i+1 < len(src) && src[i] == '[' && src[i+1] == '[' {
			result.WriteByte(' ')
			i += 2
			for i+1 < len(src) {
				if src[i] == ']' && src[i+1] == ']' {
					i += 2
					break
				}
				if src[i] == '\n' {
					result.WriteByte('\n')
				} else {
					result.WriteByte(' ')
				}
				i++
			}
			continue
		}

		result.WriteByte(src[i])
		i++
	}
	return result.String()
}
