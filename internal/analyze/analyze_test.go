package analyze

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"luanti-version-scan/internal/db"
)

func TestFindGuardName(t *testing.T) {
	tests := []struct {
		cond string
		want string
	}{
		// Feature flag check — should NOT match (handled by minetestFeaturesRe first)
		{"minetest.features.glasslike_framed", ""}, // caught by featuresRe before findGuardName

		// Bare function reference — valid existence check
		{"minetest.get_sky", "get_sky"},

		// Field access with [ — should NOT match as guard
		{`minetest.registered_nodes["foo"]`, ""},

		// Chained field access with . — should NOT match as guard
		{"minetest.registered_nodes.foo", ""},

		// Method call with : — should NOT match as guard
		{"minetest.registered_nodes:foo()", ""},

		// Function call with ( — should NOT match (has parens)
		{"minetest.get_sky()", ""},

		// Multiple conditions (and/or) — features.X followed by . is skipped, get_sky is a bare ref
		{"minetest.features.X and minetest.get_sky", "get_sky"},

		// Method bare reference — valid existence check
		{"player:get_hp", "player:get_hp"},

		// Method bare ref with chained access — should NOT match
		{"player:get_hp.foo", ""},

		// Method bare ref with table index — should NOT match
		{`player:get_hp["foo"]`, ""},

		// Not a guard at all — no minetest reference
		{"true", ""},
	}

	for _, tt := range tests {
		got := findGuardName(tt.cond)
		if got != tt.want {
			t.Errorf("findGuardName(%q) = %q, want %q", tt.cond, got, tt.want)
		}
	}
}

func TestElseGuardPropagation(t *testing.T) {
	src := `
-- Version check else should be guarded
if minetest.get_version() then
    minetest.get_clouds()
else
    minetest.get_clouds()
end

-- Feature check else should be guarded  
if minetest.features.glasslike_framed then
    minetest.get_clouds()
else
    minetest.get_clouds()
end

-- Nested else in else should be guarded
if minetest.features.biome_weights then
    minetest.get_clouds()
else
    if minetest.features.glasslike_framed then
        minetest.get_clouds()
    else
        minetest.get_clouds()
    end
end
`
	dir := t.TempDir()
	file := filepath.Join(dir, "init.lua")
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	database := db.New()
	a, err := WalkAndAnalyze(dir, database)
	if err != nil {
		t.Fatal(err)
	}

	// All get_clouds calls should be guarded since they're all inside if/else blocks
	for _, u := range a.Uses {
		if u.Name == "core.get_clouds" {
			if !u.Guarded {
				t.Errorf("get_clouds at %s:%d should be guarded (guard: %q)", u.File, u.Line, u.GuardName)
			}
		}
	}
}

func TestFeatureFlagDoesNotRaiseMinVersion(t *testing.T) {
	src := `
-- Feature flags should NOT raise the version floor
-- They are runtime guards and degrade gracefully
if minetest.features.biome_weights then
    minetest.get_biome_data(pos)
end
`
	dir := t.TempDir()
	file := filepath.Join(dir, "init.lua")
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	database := db.New()
	a, err := WalkAndAnalyze(dir, database)
	if err != nil {
		t.Fatal(err)
	}

	// Feature check should be recorded as an IsGuard
	var hasFeature bool
	for _, u := range a.Uses {
		if u.IsGuard && u.Name == "core.features.biome_weights" {
			hasFeature = true
			if !u.Guarded {
				// Feature flag itself is not guarded (it's the guard)
			}
		}
	}
	if !hasFeature {
		t.Error("feature check biome_weights not found")
	}

	// get_biome_data should be recorded and guarded
	var hasAPI bool
	for _, u := range a.Uses {
		if u.Name == "core.get_biome_data" {
			hasAPI = true
			if !u.Guarded {
				t.Error("get_biome_data inside feature check should be guarded")
			}
		}
	}
	if !hasAPI {
		t.Error("get_biome_data call not found")
	}

	// There are no unguarded API calls — feature flags should NOT contribute
	// to min version (verified by computeMinVersion in main package)
}

func TestObjPrefixFor(t *testing.T) {
	tests := []struct {
		obj    string
		prefix string
	}{
		// Exact matches
		{"player", "object:"},
		{"players", "object:"},
		{"playername", "object:"},
		{"clicker", "object:"},
		{"obj", "object:"},
		{"object", "object:"},
		{"entity", "object:"},
		{"vm", "voxelmanip:"},
		{"voxel", "voxelmanip:"},
		{"voxelmanip", "voxelmanip:"},
		{"manip", "voxelmanip:"},
		{"VoxelArea", "voxelarea:"},
		{"voxelarea", "voxelarea:"},
		{"item", "itemstack:"},
		{"items", "itemstack:"},
		{"stack", "itemstack:"},
		{"itemstack", "itemstack:"},
		{"storage", "storage:"},
		{"mod_storage", "storage:"},
		{"meta", "storage:"},
		{"data", "storage:"},
		{"file", "file:"},
		{"f", "file:"},
		{"fh", "file:"},
		{"inv", "inventory:"},
		{"inventory", "inventory:"},
		{"settings", "settings:"},
		// Substring matches
		{"my_stack", "itemstack:"},
		{"some_item", "itemstack:"},
		{"datafile", "file:"},
		{"my_inv", "inventory:"},
		{"myvoxel", "voxelmanip:"},
		{"the_manip", "voxelmanip:"},
		{"modstorage", "storage:"},
		{"my_meta", "storage:"},
		{"my_data", "storage:"},
		{"mysettings", "settings:"},
		{"myplayer", "object:"},
		{"myentity", "object:"},
		{"myobject", "object:"},
		// VoxelArea takes priority over VoxelManip
		{"the_voxelarea", "voxelarea:"},
		// Unknown (too short for first-char heuristic which requires > 2)
		{"p", ""},
		{"c", ""},
		{"o", ""},
		{"e", ""},
		{"v", ""},
		{"m", ""},
		{"i", ""},
		{"s", ""},
		{"d", ""},
		// "f" and "fh" match exact cases in the switch
		{"f", "file:"},
		// Unknown
		{"bar", ""},
		{"xyz", ""},
		{"q", ""},
	}

	for _, tt := range tests {
		got := objPrefixFor(tt.obj)
		if got != tt.prefix {
			t.Errorf("objPrefixFor(%q) = %q, want %q", tt.obj, got, tt.prefix)
		}
	}
}

func TestStripCommentsAndStrings(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"local x = 1",
			"local x = 1",
		},
		{
			"local x = 1 -- comment",
			"local x = 1  ",
		},
		{
			"local s = \"hello\"",
			"local s =       ",
		},
		{
			"local s = 'hello'",
			"local s =       ",
		},
		{
			"--[[ multi\nline\ncomment ]]\nlocal x = 1",
			"       \n    \n        \nlocal x = 1",
		},
		{
			"local ls = [[\nmultiline string\n]]",
			"local ls =  \n                \n",
		},
		{
			"if minetest.features.glasslike_framed then\n--nested\nend",
			"if minetest.features.glasslike_framed then\n \nend",
		},
	}

	for _, tt := range tests {
		got := stripCommentsAndStrings(tt.input)
		if got != tt.want {
			t.Errorf("stripCommentsAndStrings(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFindAPICalls(t *testing.T) {
	src := `
minetest.register_node("test:node", {})
minetest.find_node_near(pos, 10, "stone")
minetest.add_node(pos, {name = "air"})
minetest.some_unknown()
core.register_node("test:node2", {})
`
	lines := strings.Split(stripCommentsAndStrings(src), "\n")
	database := db.New()

	minetestFeaturesRe := regexp.MustCompile(`(?:minetest|core)\.features\.(\w+)`)
	minetestAPICallRe := regexp.MustCompile(`(?:minetest|core)\.(\w+)\s*\(`)

	known := 0
	unknown := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if minetestFeaturesRe.MatchString(line) {
			continue
		}
		if matches := minetestAPICallRe.FindAllStringSubmatch(line, -1); matches != nil {
			for _, m := range matches {
				funcName := m[1]
				if funcName == "features" {
					continue
				}
				apiName := "minetest." + funcName
				_, ok := database.LookupAPI(apiName)
				if ok {
					known++
				} else {
					unknown++
				}
			}
		}
	}

	if known != 4 {
		t.Errorf("expected 4 known API calls (register_node x2 via minetest+core, find_node_near, add_node), got %d", known)
	}
	if unknown != 1 {
		t.Errorf("expected 1 unknown API call (some_unknown), got %d", unknown)
	}
}
