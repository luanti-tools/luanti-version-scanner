package db

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		major int
		minor int
		patch int
		ok    bool
	}{
		{"5.0.0", 5, 0, 0, true},
		{"0.4.0", 0, 4, 0, true},
		{"5.16.1", 5, 16, 1, true},
		{"5.10", 5, 10, 0, true},
		{"0.4", 0, 4, 0, true},
		{"", 0, 0, 0, false},
		{"abc", 0, 0, 0, false},
		{"5", 0, 0, 0, false},
		{"5.0.0.0", 0, 0, 0, false},
	}
	for _, tt := range tests {
		v, ok := ParseVersion(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseVersion(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			continue
		}
		if ok {
			if v.Major != tt.major || v.Minor != tt.minor || v.Patch != tt.patch {
				t.Errorf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d",
					tt.input, v.Major, v.Minor, v.Patch, tt.major, tt.minor, tt.patch)
			}
		}
	}
}

func TestVersionComparison(t *testing.T) {
	a, _ := ParseVersion("5.5.0")
	b, _ := ParseVersion("5.6.0")
	c, _ := ParseVersion("5.5.1")
	d, _ := ParseVersion("0.4.16")
	e, _ := ParseVersion("5.0.0")

	if !a.LessThan(b) {
		t.Error("5.5.0 should be less than 5.6.0")
	}
	if b.LessThan(a) {
		t.Error("5.6.0 should not be less than 5.5.0")
	}
	if !a.LessThan(c) {
		t.Error("5.5.0 should be less than 5.5.1 (patch)")
	}
	if !c.GreaterThan(a) {
		t.Error("5.5.1 should be greater than 5.5.0")
	}
	if !a.GreaterThan(d) {
		t.Error("5.5.0 should be greater than 0.4.16")
	}
	if !d.LessThan(e) {
		t.Error("0.4.16 should be less than 5.0.0")
	}
}

func TestDBLoads(t *testing.T) {
	db := New()
	if db == nil {
		t.Fatal("New() returned nil")
	}
	if len(db.APIs) == 0 {
		t.Error("db.APIs is empty")
	}
	if len(db.Features) == 0 {
		t.Error("db.Features is empty")
	}
}

func TestLookupAPI(t *testing.T) {
	db := New()

	tests := []struct {
		name    string
		wantOk  bool
		wantVer string
	}{
		{"minetest.register_node", true, "0.4.0"},
		{"minetest.add_node", true, "0.4.0"},
		{"minetest.get_version", true, "0.4.15"},
		{"object:get_pos", true, "0.4.16"},
		{"voxelmanip:get_data", true, "0.4.8"},
		{"voxelarea:new", true, "0.4.8"},
		{"settings:get", true, "0.4.8"},
		{"minetest.nonexistent", false, ""},
		{"object:fly", false, ""},
	}

	for _, tt := range tests {
		entry, ok := db.LookupAPI(tt.name)
		if ok != tt.wantOk {
			t.Errorf("LookupAPI(%q) ok = %v, want %v", tt.name, ok, tt.wantOk)
			continue
		}
		if ok && entry.Version != tt.wantVer {
			t.Errorf("LookupAPI(%q) version = %q, want %q", tt.name, entry.Version, tt.wantVer)
		}
	}
}

func TestLookupFeature(t *testing.T) {
	db := New()

	tests := []struct {
		name    string
		wantOk  bool
		wantVer string
	}{
		{"use_texture_alpha", true, "0.4.7"},
		{"biome_weights", true, "5.11.0"},
		{"bulk_lbms", true, "5.10.0"},
		{"chunksize_vector", true, "5.15.0"},
		{"fake_feature", false, ""},
	}

	for _, tt := range tests {
		entry, ok := db.LookupFeature(tt.name)
		if ok != tt.wantOk {
			t.Errorf("LookupFeature(%q) ok = %v, want %v", tt.name, ok, tt.wantOk)
			continue
		}
		if ok && entry.Version != tt.wantVer {
			t.Errorf("LookupFeature(%q) version = %q, want %q", tt.name, entry.Version, tt.wantVer)
		}
	}
}

func TestAllAPIVersions(t *testing.T) {
	db := New()
	versions := db.AllAPIVersions()
	if len(versions) == 0 {
		t.Fatal("AllAPIVersions() returned empty")
	}

	for i := 1; i < len(versions); i++ {
		va, _ := ParseVersion(versions[i-1])
		vb, _ := ParseVersion(versions[i])
		if !va.LessThan(vb) {
			t.Errorf("Versions not sorted: %s >= %s", versions[i-1], versions[i])
		}
	}
}

func TestVersionString(t *testing.T) {
	v, _ := ParseVersion("5.10.0")
	if v.String() != "5.10.0" {
		t.Errorf("Version.String() = %q, want %q", v.String(), "5.10.0")
	}

	v2, _ := ParseVersion("0.4")
	if v2.String() != "0.4.0" {
		t.Errorf("Version.String() = %q, want %q", v2.String(), "0.4.0")
	}
}
