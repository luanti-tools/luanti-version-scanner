package db

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed db.json
var dbJSON []byte

type Version struct {
	Major, Minor, Patch int
}

func ParseVersion(s string) (Version, bool) {
	var v Version
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return v, false
	}
	_, err := fmt.Sscanf(parts[0], "%d", &v.Major)
	if err != nil {
		return v, false
	}
	_, err = fmt.Sscanf(parts[1], "%d", &v.Minor)
	if err != nil {
		return v, false
	}
	if len(parts) == 3 {
		_, err = fmt.Sscanf(parts[2], "%d", &v.Patch)
		if err != nil {
			return v, false
		}
	}
	return v, true
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) LessThan(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

func (v Version) GreaterThan(other Version) bool {
	return other.LessThan(v)
}

func (v Version) IsZero() bool {
	return v.Major == 0 && v.Minor == 0 && v.Patch == 0
}

type ParamEntry struct {
	Version    string `json:"Version"`
	MaxVersion string `json:"MaxVersion,omitempty"`
}

type Entry struct {
	Version    string                `json:"Version"`
	MaxVersion string                `json:"MaxVersion,omitempty"`
	Params     map[string]ParamEntry `json:"Params,omitempty"`
}

type FileFormatEntry struct {
	Version string `json:"Version"`
}

type DB struct {
	APIs             map[string]Entry
	Features         map[string]Entry
	FileFormats      map[string]FileFormatEntry
	FormspecVersions map[int]string
}

func New() *DB {
	var data struct {
		APIs             map[string]Entry           `json:"apis"`
		Features         map[string]Entry           `json:"features"`
		FileFormats      map[string]FileFormatEntry `json:"file_formats"`
		FormspecVersions map[string]string          `json:"formspec_versions"`
	}
	if err := json.Unmarshal(dbJSON, &data); err != nil {
		panic("failed to parse embedded db.json: " + err.Error())
	}
	db := &DB{
		APIs:        data.APIs,
		Features:    data.Features,
		FileFormats: data.FileFormats,
	}
	if db.APIs == nil {
		db.APIs = make(map[string]Entry)
	}
	if db.Features == nil {
		db.Features = make(map[string]Entry)
	}
	if db.FileFormats == nil {
		db.FileFormats = make(map[string]FileFormatEntry)
	}
	db.FormspecVersions = make(map[int]string)
	for k, v := range data.FormspecVersions {
		n, err := strconv.Atoi(k)
		if err == nil {
			db.FormspecVersions[n] = v
		}
	}
	return db
}

func (db *DB) LookupAPI(name string) (Entry, bool) {
	e, ok := db.APIs[name]
	return e, ok
}

func (db *DB) LookupFeature(name string) (Entry, bool) {
	e, ok := db.Features[name]
	return e, ok
}

func (db *DB) LookupFileFormat(ext string) (FileFormatEntry, bool) {
	e, ok := db.FileFormats[ext]
	return e, ok
}

func (db *DB) LookupFormspecVersion(ver int) (string, bool) {
	v, ok := db.FormspecVersions[ver]
	return v, ok
}

func (db *DB) AllAPIVersions() []string {
	seen := make(map[string]bool)
	for _, e := range db.APIs {
		seen[e.Version] = true
	}
	var versions []string
	for v := range seen {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		vi, _ := ParseVersion(versions[i])
		vj, _ := ParseVersion(versions[j])
		return vi.LessThan(vj)
	})
	return versions
}
