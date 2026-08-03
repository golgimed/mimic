// Package openapi is an adapter that turns OpenAPI 3.x specs into a
// registry.Provider, so the mock server can serve routes generated from a
// spec exactly like it serves any hand-written provider. Route/ResponsePlan
// are private to this package — core and registry never know a route came
// from a spec.
package openapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// doc is the minimal subset of an OpenAPI 3.x document this adapter
// understands: paths, operations, and their responses' examples/schemas.
// Anything else in the spec (security schemes, components other than
// inline schemas, servers, etc.) is ignored.
//
// This is intentionally frozen at OpenAPI 3.0-shaped semantics pending a
// timeboxed spike into an actual parsing library (kin-openapi vs.
// pb33f/libopenapi) for proper 3.1 / JSON Schema 2020-12 and $ref support.
// Don't grow doc/schema further without that spike — see the OpenAPI
// providers plan (parsing-library issue) for the trade-offs already
// considered.
type doc struct {
	Info struct {
		Title   string `yaml:"title"`
		Version string `yaml:"version"`
	} `yaml:"info"`
	// MimicName overrides the auto-derived (slugified title) route prefix
	// for this spec. Optional; standard OpenAPI vendor-extension escape
	// hatch, used to resolve prefix collisions without a sidecar file.
	MimicName string `yaml:"x-mimic-name"`
	// MimicPersist overrides MIMIC_OPENAPI_PERSIST for this spec's
	// CRUD-shaped routes. Optional; nil means "use the global default".
	MimicPersist *bool                           `yaml:"x-mimic-persist"`
	Paths        map[string]map[string]operation `yaml:"paths"`
}

type operation struct {
	OperationID string              `yaml:"operationId"`
	Responses   map[string]response `yaml:"responses"`
	// Behavior is the x-mimic-behavior vendor extension: a declarative,
	// per-operation alternative to configuring a fault via PUT /admin/faults
	// after the fact. Optional; nil means no spec-declared behavior for this
	// operation. See behavior.go for how it's seeded into fault_config.
	Behavior *behaviorSpec `yaml:"x-mimic-behavior"`
}

type response struct {
	Content map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema   *schema            `yaml:"schema"`
	Example  any                `yaml:"example"`
	Examples map[string]example `yaml:"examples"`
}

type example struct {
	Value any `yaml:"value"`
}

type schema struct {
	Type       string             `yaml:"type"`
	Format     string             `yaml:"format"`
	Properties map[string]*schema `yaml:"properties"`
	Items      *schema            `yaml:"items"`
	Example    any                `yaml:"example"`
	Enum       []any              `yaml:"enum"`
}

var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true,
}

// LoadedSpec is a parsed spec ready to be converted into routes.
type LoadedSpec struct {
	Path      string
	Title     string
	Version   string // info.version, empty if unset
	Checksum  string // sha256 of the raw spec file, hex-encoded
	MimicName string // x-mimic-name override, empty if unset
	Persist   *bool  // x-mimic-persist override, nil if unset
	// Warnings lists constructs this adapter found in the spec but doesn't
	// understand (see unsupportedConstructKeys) — affected schemas silently
	// fall back to stub/empty bodies unless the caller surfaces this.
	Warnings []string
	doc      doc
}

// unsupportedConstructKeys are OpenAPI/JSON Schema keys this adapter's doc
// struct has no field for (see the comment on doc above) and therefore
// silently drops via yaml.v3's unknown-field-ignore behavior.
var unsupportedConstructKeys = []string{"$ref", "oneOf", "allOf", "anyOf"}

// findUnsupportedConstructs walks a generically-decoded spec looking for any
// key in unsupportedConstructKeys, returning the distinct ones found.
func findUnsupportedConstructs(node any) []string {
	found := make(map[string]bool)
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			for _, key := range unsupportedConstructKeys {
				if _, ok := v[key]; ok {
					found[key] = true
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(node)

	keys := make([]string, 0, len(found))
	for _, k := range unsupportedConstructKeys {
		if found[k] {
			keys = append(keys, k)
		}
	}
	return keys
}

// Discover recursively finds spec files under dir (if set) and unions in
// every path matching each pattern in globs (e.g. "specs/**/*.yaml"). Go's
// filepath.Glob doesn't support "**", so a "**" segment is treated as
// "search all subdirectories from here down".
func Discover(dir string, globs []string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string

	add := func(path string) {
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}

	if dir != "" {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				slog.Warn("openapi: skipping unreadable path during spec discovery", "path", path, "error", err)
				return nil
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".yaml" || ext == ".yml" || ext == ".json" {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("discover specs in %q: %w", dir, err)
		}
	}

	for _, glob := range globs {
		pattern := glob
		root := "."
		if idx := strings.Index(glob, "**"); idx >= 0 {
			if idx > 0 {
				root = strings.TrimSuffix(glob[:idx], "/")
			}
			pattern = strings.TrimPrefix(glob[idx:], "**/")

			err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
					add(path)
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("discover specs matching %q: %w", glob, err)
			}
			continue
		}

		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("discover specs matching %q: %w", glob, err)
		}
		for _, m := range matches {
			add(m)
		}
	}

	sort.Strings(out)
	return out, nil
}

// Load parses a single spec file (YAML or JSON — encoding/json is a subset
// of YAML, so one decoder handles both).
func Load(path string) (*LoadedSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec %q: %w", path, err)
	}

	var d doc
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("parse spec %q: %w", path, err)
	}

	var generic any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("parse spec %q: %w", path, err)
	}
	var warnings []string
	for _, construct := range findUnsupportedConstructs(generic) {
		warnings = append(warnings, fmt.Sprintf(
			"spec uses %q, which this adapter does not resolve — affected schemas fall back to stub/empty bodies",
			construct))
	}

	sum := sha256.Sum256(raw)
	return &LoadedSpec{
		Path:      path,
		Title:     d.Info.Title,
		Version:   d.Info.Version,
		Checksum:  hex.EncodeToString(sum[:]),
		MimicName: d.MimicName,
		Persist:   d.MimicPersist,
		Warnings:  warnings,
		doc:       d,
	}, nil
}

// LoadAll discovers and parses every spec under dir and/or matching globs.
func LoadAll(dir string, globs []string) ([]*LoadedSpec, error) {
	paths, err := Discover(dir, globs)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no spec files found (dir=%q globs=%q)", dir, globs)
	}

	specs := make([]*LoadedSpec, 0, len(paths))
	for _, p := range paths {
		spec, err := Load(p)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}
