// Package openapi is an adapter that turns OpenAPI 3.x specs into a
// registry.Provider, so the mock server can serve routes generated from a
// spec exactly like it serves any hand-written provider. Route/ResponsePlan
// are private to this package — core and registry never know a route came
// from a spec.
package openapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// doc is this adapter's internal model of an OpenAPI 3.x document: paths,
// operations, their parameters/request bodies, and their responses'
// examples/schemas. It is built from a kin-openapi document (see
// fromKinDoc in translate.go) so kin-openapi types never leave this file
// and translate.go — the rest of the package, and the rest of Mimic, only
// see these structs.
type doc struct {
	Title   string
	Version string
	// MimicName overrides the auto-derived (slugified title) route prefix
	// for this spec. Optional; standard OpenAPI vendor-extension escape
	// hatch, used to resolve prefix collisions without a sidecar file.
	MimicName string
	// MimicPersist overrides MIMIC_OPENAPI_PERSIST for this spec's
	// CRUD-shaped routes. Optional; nil means "use the global default".
	MimicPersist *bool
	Paths        map[string]map[string]operation
}

type operation struct {
	OperationID string
	Parameters  []parameter
	RequestBody *requestBody
	Responses   map[string]response
	// Behavior is the x-mimic-behavior vendor extension: a declarative,
	// per-operation alternative to configuring a fault via PUT /admin/faults
	// after the fact. Optional; nil means no spec-declared behavior for this
	// operation. See behavior.go for how it's seeded into fault_config.
	Behavior *behaviorSpec
}

// parameter is one path/query/header/cookie parameter, with any $ref
// already resolved.
type parameter struct {
	Name     string
	In       string
	Required bool
	Schema   *schema
}

type requestBody struct {
	Required bool
	Content  map[string]mediaType
}

type response struct {
	Content map[string]mediaType
}

type mediaType struct {
	Schema   *schema
	Example  any
	Examples map[string]example
}

type example struct {
	Value any
}

// schema is the subset of JSON Schema the response generator understands.
// $ref is resolved at load time, so a schema graph may be cyclic for
// recursive definitions — consumers must guard against unbounded descent.
// allOf members are merged into the schema itself at translation time;
// oneOf/anyOf alternatives are kept in Variants.
type schema struct {
	Type       string
	Format     string
	Properties map[string]*schema
	Items      *schema
	Example    any
	Enum       []any
	Variants   []*schema
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
	// Warnings lists problems found while validating the spec against the
	// OpenAPI 3.x schema. They don't stop the spec from loading (a sloppy
	// but usable spec still yields routes), but are logged at boot.
	Warnings []string
	doc      doc
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
		found, err := discoverInDir(dir)
		if err != nil {
			return nil, err
		}
		for _, p := range found {
			add(p)
		}
	}

	for _, glob := range globs {
		found, err := discoverGlob(glob)
		if err != nil {
			return nil, err
		}
		for _, p := range found {
			add(p)
		}
	}

	sort.Strings(out)
	return out, nil
}

// discoverInDir recursively finds .yaml/.yml/.json files under dir.
func discoverInDir(dir string) ([]string, error) {
	var out []string
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
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover specs in %q: %w", dir, err)
	}
	return out, nil
}

// discoverGlob resolves one glob pattern. A "**" segment means "search all
// subdirectories from here down" (filepath.Glob doesn't support "**"
// natively); anything else is a plain filepath.Glob pattern.
func discoverGlob(glob string) ([]string, error) {
	idx := strings.Index(glob, "**")
	if idx < 0 {
		matches, err := filepath.Glob(glob)
		if err != nil {
			return nil, fmt.Errorf("discover specs matching %q: %w", glob, err)
		}
		return matches, nil
	}

	root := "."
	if idx > 0 {
		root = strings.TrimSuffix(glob[:idx], "/")
	}
	pattern := strings.TrimPrefix(glob[idx:], "**/")

	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover specs matching %q: %w", glob, err)
	}
	return out, nil
}

// Load parses a single spec file (YAML or JSON) with kin-openapi, resolving
// local $refs. Structural errors (unparseable document, unresolvable $ref)
// fail the load; OpenAPI schema-validation findings are kept as Warnings.
func Load(path string) (*LoadedSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec %q: %w", path, err)
	}

	loader := openapi3.NewLoader()
	// External (file/URL) $refs are intentionally unsupported: a spec must
	// be self-contained so loading never touches the network or other files.
	loader.IsExternalRefsAllowed = false
	kdoc, err := loader.LoadFromData(raw)
	if err != nil {
		return nil, fmt.Errorf("parse spec %q: %w", path, err)
	}

	var warnings []string
	if err := kdoc.Validate(context.Background(), openapi3.DisableExamplesValidation()); err != nil {
		warnings = append(warnings, "spec does not fully conform to OpenAPI 3.x: "+err.Error())
	}

	d, err := fromKinDoc(kdoc)
	if err != nil {
		return nil, fmt.Errorf("translate spec %q: %w", path, err)
	}

	sum := sha256.Sum256(raw)
	return &LoadedSpec{
		Path:      path,
		Title:     d.Title,
		Version:   d.Version,
		Checksum:  hex.EncodeToString(sum[:]),
		MimicName: d.MimicName,
		Persist:   d.MimicPersist,
		Warnings:  warnings,
		doc:       d,
	}, nil
}

// decodeExtension re-decodes a kin-openapi vendor-extension value (already
// generically decoded) into out, via YAML so out's yaml tags apply.
func decodeExtension(ext map[string]any, key string, out any) (bool, error) {
	v, ok := ext[key]
	if !ok || v == nil {
		return false, nil
	}
	raw, err := yaml.Marshal(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	if err := yaml.Unmarshal(raw, out); err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return true, nil
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
