package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"luanti-version-scan/internal/analyze"

	"luanti-version-scan/internal/db"
	"golang.org/x/term"
)

var useColor = term.IsTerminal(int(os.Stdout.Fd()))

func init() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `Luanti Version Scanner

Scan Luanti mods to determine minimum and maximum supported versions
Supports single mods, modpacks (modpack.conf), and games (game.conf)

Usage: luanti-version-scan [OPTIONS] [<directory>]

Arguments:
  <directory>  Path to the mod, modpack, or game directory to scan

Options:
  -d, --dir <directory>  Path to the mod, modpack, or game directory to scan
  -l, --list             List all detected API calls
  -v, --verbose          Verbose output
  -w, --write            Write min_minetest_version and max_minetest_version to .conf
  -q, --quiet            Only output min and max versions
  -h, --help             Print help
`)
	}
}

func main() {
	dir := flag.String("d", "", "")
	flag.StringVar(dir, "dir", "", "")
	listAll := flag.Bool("l", false, "")
	flag.BoolVar(listAll, "list", false, "")
	doWrite := flag.Bool("w", false, "")
	flag.BoolVar(doWrite, "write", false, "")
	quiet := flag.Bool("q", false, "")
	flag.BoolVar(quiet, "quiet", false, "")
	verbose := flag.Bool("v", false, "")
	flag.BoolVar(verbose, "verbose", false, "")
	flag.Parse()

	modDir := *dir
	if modDir == "" {
		if flag.NArg() > 0 {
			modDir = flag.Arg(0)
		} else {
			modDir = detectModDir()
		}
	}

	info, err := os.Stat(modDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", modDir)
		os.Exit(1)
	}

	database := db.New()

	if *verbose {
		*listAll = true
	}

	if !*quiet {
		printHeader()
	}

	if isGameDir(modDir) {
		modsDir := filepath.Join(modDir, "mods")
		analyzeCollection(modDir, modsDir, "Game", "game.conf", database, *listAll, *verbose, *quiet, *doWrite)
	} else if isModPack(modDir) {
		analyzeCollection(modDir, modDir, "Modpack", "modpack.conf", database, *listAll, *verbose, *quiet, *doWrite)
	} else {
		analyzeMod(modDir, filepath.Base(modDir), database, *listAll, *verbose, *quiet, *doWrite)
	}
}

func detectModDir() string {
	dir, _ := os.Getwd()
	for {
		if isModDir(dir) {
			return dir
		}
		if isModPack(dir) || isGameDir(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	fmt.Fprintf(os.Stderr, "Error: no mod or modpack directory found.\n")
	fmt.Fprintf(os.Stderr, "Usage: luanti-version-scan [path-to-mod-directory]\n")
	os.Exit(1)
	return ""
}

func analyzeCollection(packDir, modsDir, label, confFile string, database *db.DB, listAll, verbose, quiet, doWrite bool) {
	mods := listMods(modsDir)
	if len(mods) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no mods found in %s.\n", label)
		os.Exit(1)
	}
	var packMin, packMax *db.Version
	for _, mod := range mods {
		path := filepath.Join(modsDir, mod)
		if !quiet {
			fmt.Printf("\n  ===== %s =====\n", mod)
		}
		minV, maxV := analyzeMod(path, mod, database, listAll, verbose, quiet, doWrite)
		if packMin == nil || (minV != nil && packMin.LessThan(*minV)) {
			packMin = minV
		}
		if packMax == nil || (maxV != nil && maxV.LessThan(*packMax)) {
			packMax = maxV
		}
	}

	if quiet {
		if packMin != nil {
			fmt.Printf("%s: min_minetest_version: %s\n", label, packMin)
		} else {
			fmt.Printf("%s: min_minetest_version: unknown\n", label)
		}
		if packMax != nil {
			fmt.Printf("%s: max_minetest_version: %s\n", label, packMax)
		}
	} else {
		fmt.Printf("\n  ===== %s Summary =====\n", label)
		fmt.Printf("  min_minetest_version: ")
		if packMin != nil {
			fmt.Printf("%s\n", packMin)
		} else {
			fmt.Println("unknown")
		}
		if packMax != nil {
			fmt.Printf("  max_minetest_version: %s\n", packMax)
		}
		fmt.Println()
	}

	if doWrite {
		writeConf(packDir, confFile, packMin, packMax)
	}
}

func analyzeMod(path, displayName string, database *db.DB, listAll, verbose, quiet, doWrite bool) (*db.Version, *db.Version) {
	analysis, err := analyze.WalkAndAnalyze(path, database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	minVersion := computeMinVersion(analysis, database)
	maxVersion := computeMaxVersion(analysis)

	if quiet {
		if minVersion != nil {
			fmt.Printf("%s: min_minetest_version: %s\n", displayName, minVersion)
		} else {
			fmt.Printf("%s: min_minetest_version: unknown\n", displayName)
		}
		if maxVersion != nil {
			fmt.Printf("%s: max_minetest_version: %s\n", displayName, maxVersion)
		}
	} else {
		printReport(analysis, path, displayName, minVersion, maxVersion, listAll, verbose)
	}

	if doWrite {
		writeConf(path, "mod.conf", minVersion, maxVersion)
	}

	return minVersion, maxVersion
}

func isGameDir(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, "game.conf"))
	return err == nil && !fi.IsDir()
}

func isModPack(dir string) bool {
	if fi, err := os.Stat(filepath.Join(dir, "modpack.conf")); err == nil && !fi.IsDir() {
		return true
	}
	if fi, err := os.Stat(filepath.Join(dir, "modpack.txt")); err == nil && !fi.IsDir() {
		return true
	}
	return false
}

func listMods(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var mods []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		if isModDir(sub) {
			mods = append(mods, e.Name())
		}
	}
	sort.Strings(mods)
	return mods
}

func isModDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "mod.conf"))
	if err == nil && !info.IsDir() {
		return true
	}
	info, err = os.Stat(filepath.Join(dir, "init.lua"))
	if err == nil && !info.IsDir() {
		return true
	}
	info, err = os.Stat(filepath.Join(dir, "depends.txt"))
	if err == nil && !info.IsDir() {
		return true
	}
	return false
}

func printHeader() {
	fmt.Println(strings.Repeat("═", 50))
	fmt.Println("  Luanti Version Scanner")
	fmt.Println(strings.Repeat("═", 50))
	fmt.Println()
}

func printReport(a *analyze.Analysis, dir, displayName string, minVersion, maxVersion *db.Version, listAll, verbose bool) {
	// mod.conf info
	if a.MinetestVersion != "" {
		fmt.Printf("  mod.conf:  minetest_version = %s\n", a.MinetestVersion)
	}
	if a.FormspecVersion > 0 {
		fmt.Printf("  mod.conf:  formspec_version = %d\n", a.FormspecVersion)
	}
	if a.MinetestVersion != "" || a.FormspecVersion > 0 {
		fmt.Println()
	}

	// File formats
	if len(a.FileFormats) > 0 || verbose {
		fmt.Println("  File formats:")
		if len(a.FileFormats) > 0 {
			for _, ff := range a.FileFormats {
				fmt.Printf("    %s → %s\n", ff.Ext, ff.Version)
			}
		} else {
			fmt.Println("    none")
		}
		fmt.Println()
	}

	// Group uses
	var unguarded, guarded, featureChecks, unknown []analyze.FeatureUse

	for _, u := range a.Uses {
		if !u.Known {
			unknown = append(unknown, u)
			continue
		}
		if u.IsGuard {
			featureChecks = append(featureChecks, u)
			continue
		}
		if u.Guarded {
			guarded = append(guarded, u)
		} else {
			unguarded = append(unguarded, u)
		}
	}

	// Deduplicate and sort
	unguarded = dedupSort(unguarded)
	guarded = dedupSort(guarded)
	featureChecks = dedupSort(featureChecks)
	unknown = dedupSort(unknown)

	// Print unguarded
	{
		highest := ""
		if minVersion != nil {
			highest = minVersion.String()
		}
		lowestMax := ""
		if maxVersion != nil {
			lowestMax = maxVersion.String()
		}
		showCount := len(unguarded)
		if !listAll {
			cnt := 0
			for _, u := range unguarded {
				ev := effectiveVersion(u)
				if ev == highest || u.MaxVersion != "" {
					cnt++
				}
			}
			showCount = cnt
		}
		if showCount > 0 {
			fmt.Println("  Unguarded API calls:")
			for _, u := range unguarded {
				ev := effectiveVersion(u)
				if !listAll && !verbose && ev != highest && u.MaxVersion == "" {
					continue
				}
				mark := "✓"
				if useColor {
					mark = "\033[32m✓\033[0m"
				}
				if ev == highest {
					if useColor {
						mark = "\033[31m▲\033[0m"
					} else {
						mark = "▲"
					}
				}
				dep := ""
				if u.MaxVersion != "" {
					dep = fmt.Sprintf(" [deprecated since %s]", u.MaxVersion)
				}
				fmt.Printf("  %s %s → %s%s\n", mark, boldName(u.Name, 32), u.Version, dep)
				if verbose || ev == highest || (u.MaxVersion != "" && u.MaxVersion == lowestMax) {
					fmt.Printf("    └── %s:%d\n", u.File, u.Line)
				}
			}
			fmt.Println()
		}
	}

	// Print guarded
	if len(guarded) > 0 || listAll || verbose {
		if len(guarded) > 0 {
			fmt.Println("  Guarded API calls:")
			for _, u := range guarded {
				guardInfo := ""
				if u.GuardName != "" {
					guardInfo = fmt.Sprintf(" [guarded by: %s]", u.GuardName)
				}
				dep := ""
				if u.MaxVersion != "" {
					dep = fmt.Sprintf(" [deprecated since %s]", u.MaxVersion)
				}
					fmt.Printf("  ✓ %s → %s%s%s\n", boldName(u.Name, 32), u.Version, guardInfo, dep)
				if verbose {
					fmt.Printf("    └── %s:%d\n", u.File, u.Line)
				}
			}
			fmt.Println()
		} else {
			fmt.Println("  Guarded API calls:")
			fmt.Println("  none")
			fmt.Println()
		}
	}

	// Print feature checks
	if len(featureChecks) > 0 || listAll || verbose {
		if len(featureChecks) > 0 {
			fmt.Println("  Feature checks:")
			for _, u := range featureChecks {
				fmt.Printf("  ✓ %s\n", u.Name)
				if verbose {
					fmt.Printf("    └── %s:%d\n", u.File, u.Line)
				}
			}
			fmt.Println()
		} else {
			fmt.Println("  Feature checks:")
			fmt.Println("  none")
			fmt.Println()
		}
	}

	// Print unknown
	if len(unknown) > 0 {
		fmt.Println("  Unknown features:")
		for _, u := range unknown {
			g := ""
			if u.Guarded {
				g = " [guarded]"
			}
			fmt.Printf("  ? %s (%s:%d)%s\n", u.Name, u.File, u.Line, g)
		}
		fmt.Println()
	}

	coreNote := hasCorePrefixFloor(a, minVersion)
	if coreNote != "" {
		fmt.Printf("  %s\n", coreNote)
	}

	fmt.Println()
	fmt.Printf("  min_minetest_version: ")
	if minVersion != nil {
		fmt.Printf("%s\n", minVersion)
	} else {
		fmt.Println("unknown")
	}

	if maxVersion != nil {
		fmt.Printf("  max_minetest_version: %s\n", maxVersion)
	} else if verbose {
		fmt.Println("  max_minetest_version: none")
	}

	if a.MinetestVersion != "" {
		mv, ok := db.ParseVersion(a.MinetestVersion)
		if ok && minVersion != nil && mv.LessThan(*minVersion) {
			fmt.Printf("  Warning: mod.conf says %s but unguarded API calls require %s\n",
				a.MinetestVersion, minVersion)
		}
		if ok && maxVersion != nil && maxVersion.LessThan(mv) {
			fmt.Printf("  Warning: mod.conf says %s but deprecated API calls limit to %s\n",
				a.MinetestVersion, maxVersion)
		}
	}
	fmt.Println()
}

func effectiveVersion(u analyze.FeatureUse) string {
	v, ok := db.ParseVersion(u.Version)
	if ok && u.UseCorePrefix {
		coreIntroduced, _ := db.ParseVersion("0.4.10")
		if v.LessThan(coreIntroduced) {
			return "0.4.10"
		}
	}
	return u.Version
}

func hasCorePrefixFloor(a *analyze.Analysis, minVersion *db.Version) string {
	coreIntroduced, _ := db.ParseVersion("0.4.10")
	if minVersion == nil || minVersion.LessThan(coreIntroduced) || coreIntroduced.LessThan(*minVersion) {
		return ""
	}
	for _, u := range a.Uses {
		if u.IsGuard || u.Guarded || !u.Known {
			continue
		}
		v, ok := db.ParseVersion(u.Version)
		if ok && u.UseCorePrefix && v.LessThan(coreIntroduced) {
			return "Note: core prefix raises minimum version to 0.4.10"
		}
	}
	return ""
}

func writeConf(modDir, filename string, minVersion, maxVersion *db.Version) {
	confPath := filepath.Join(modDir, filename)
	data, err := os.ReadFile(confPath)
	exists := err == nil

	var lines []string
	if exists {
		lines = strings.Split(string(data), "\n")
	}

	var result []string
	minWritten := false
	maxWritten := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "min_minetest_version") {
			if minVersion != nil {
				result = append(result, fmt.Sprintf("min_minetest_version = %s", minVersion))
			} else {
				result = append(result, line)
			}
			minWritten = true
			continue
		}
		if strings.HasPrefix(trimmed, "max_minetest_version") {
			if maxVersion != nil {
				result = append(result, fmt.Sprintf("max_minetest_version = %s", maxVersion))
			} else {
				result = append(result, line)
			}
			maxWritten = true
			continue
		}
		result = append(result, line)
	}

	// Skip writing if no version info
	if minVersion == nil && maxVersion == nil {
		return
	}

	// Trim trailing empty lines before appending new entries
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	if !minWritten && minVersion != nil {
		result = append(result, fmt.Sprintf("min_minetest_version = %s", minVersion))
	}
	if !maxWritten && maxVersion != nil {
		result = append(result, fmt.Sprintf("max_minetest_version = %s", maxVersion))
	}

	output := strings.Join(result, "\n") + "\n"
	if err := os.WriteFile(confPath, []byte(output), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing mod.conf: %v\n", err)
		os.Exit(1)
	}
	var written []string
	if minVersion != nil {
		written = append(written, fmt.Sprintf("min_minetest_version = %s", minVersion))
	}
	if maxVersion != nil {
		written = append(written, fmt.Sprintf("max_minetest_version = %s", maxVersion))
	}
	if len(written) > 0 {
		fmt.Printf("  %s saved to %s\n", strings.Join(written, " and "), filename)
	}
}

func computeMinVersion(a *analyze.Analysis, database *db.DB) *db.Version {
	var maxVer *db.Version

	// Check mod.conf
	if a.MinetestVersion != "" {
		v, ok := db.ParseVersion(a.MinetestVersion)
		if ok {
			maxVer = &v
		}
	}

	coreIntroduced, _ := db.ParseVersion("0.4.10")

	for _, u := range a.Uses {
		if u.IsGuard {
			continue
		}
		if u.Guarded {
			continue
		}
		if !u.Known {
			continue
		}
		v, ok := db.ParseVersion(u.Version)
		if !ok {
			continue
		}
		// If the call used core. prefix, raise version to 0.4.10
		// (minetest was introduced as a core alias in 0.4.10)
		if u.UseCorePrefix && v.LessThan(coreIntroduced) {
			v = coreIntroduced
		}
		if maxVer == nil || v.GreaterThan(*maxVer) {
			maxVer = &v
		}
	}

	for _, ff := range a.FileFormats {
		v, ok := db.ParseVersion(ff.Version)
		if ok {
			if maxVer == nil || v.GreaterThan(*maxVer) {
				maxVer = &v
			}
		}
	}

	if a.FormspecVersion > 0 {
		if fsv, ok := database.LookupFormspecVersion(a.FormspecVersion); ok {
			if v, ok2 := db.ParseVersion(fsv); ok2 {
				if maxVer == nil || v.GreaterThan(*maxVer) {
					maxVer = &v
				}
			}
		}
	}

	return maxVer
}

func computeMaxVersion(a *analyze.Analysis) *db.Version {
	var minMaxVer *db.Version

	for _, u := range a.Uses {
		if u.IsGuard {
			continue
		}
		if u.MaxVersion == "" {
			continue
		}
		v, ok := db.ParseVersion(u.MaxVersion)
		if !ok {
			continue
		}
		if minMaxVer == nil || v.LessThan(*minMaxVer) {
			minMaxVer = &v
		}
	}

	return minMaxVer
}

func boldName(name string, width int) string {
	if len(name) > width-3 {
		name = name[:width-6] + "..."
	}
	return fmt.Sprintf("%-*s", width, name)
}

func dedupSort(uses []analyze.FeatureUse) []analyze.FeatureUse {
	seen := make(map[string]bool)
	var result []analyze.FeatureUse
	for _, u := range uses {
		key := u.Name
		if u.GuardName != "" {
			key += "@" + u.GuardName
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, u)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		vi, _ := db.ParseVersion(result[i].Version)
		vj, _ := db.ParseVersion(result[j].Version)
		if vi.String() != vj.String() {
			return vi.LessThan(vj)
		}
		return result[i].Name < result[j].Name
	})
	return result
}
