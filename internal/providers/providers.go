// Package providers wires every known provider into the registry. Adding a
// provider means creating internal/providers/<name>/ and adding one line
// here — no other file should need to change.
package providers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golgimed/mimic/internal/openapi"
	"github.com/golgimed/mimic/internal/providers/bryscad"
	"github.com/golgimed/mimic/internal/providers/integraicp"
	"github.com/golgimed/mimic/internal/providers/sncr"
	"github.com/golgimed/mimic/internal/providers/zenvia"
	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/scheduler"
)

func RegisterAll(reg *registry.Registry, db *sql.DB, faultStore *admin.Store, sched *scheduler.Scheduler, zenviaStatusDelay time.Duration, bryScadWebhookURL string) {
	reg.Register(zenvia.New(db, faultStore, sched, zenviaStatusDelay))
	reg.Register(integraicp.New(db, faultStore))
	reg.Register(sncr.New(db, faultStore))
	reg.Register(bryscad.New(db, faultStore, bryScadWebhookURL))
}

// OpenAPIOptions configures RegisterOpenAPI's spec loading/merging.
// PersistDefault is the MIMIC_OPENAPI_PERSIST default applied to every
// spec's CRUD-shaped routes, unless a spec overrides it via x-mimic-persist.
type OpenAPIOptions struct {
	SpecDir        string
	SpecGlobs      []string
	ConflictMode   openapi.ConflictMode
	PersistDefault bool
}

// RegisterOpenAPI loads every spec found under opts.SpecDir and/or matching
// opts.SpecGlobs, converts each into routes prefixed by its own slugified
// title (or its x-mimic-name override) so specs can't collide with each
// other or with hand-written providers, merges them into one route table per
// opts.ConflictMode, and registers the result as a single "openapi"
// provider. No-op if neither SpecDir nor SpecGlobs is set. Returns the
// number of specs and merged routes loaded, for startup logging.
//
// Like storage.RunMigrations, this must only be called once, at boot,
// before the mux is built — parsing never happens per-request, only here.
func RegisterOpenAPI(reg *registry.Registry, db *sql.DB, faultStore *admin.Store, opts OpenAPIOptions, log *slog.Logger) (int, int, error) {
	if opts.SpecDir == "" && len(opts.SpecGlobs) == 0 {
		return 0, 0, nil
	}

	specs, err := openapi.LoadAll(opts.SpecDir, opts.SpecGlobs)
	if err != nil {
		return 0, 0, fmt.Errorf("load openapi specs: %w", err)
	}
	logSpecWarnings(log, specs)

	specRoutes, metas, err := buildSpecRoutes(reg, specs, opts.PersistDefault)
	if err != nil {
		return 0, 0, err
	}

	routes, err := openapi.Merge(specRoutes, opts.ConflictMode)
	if err != nil {
		return 0, 0, fmt.Errorf("merge openapi specs: %w", err)
	}

	if err := seedBehaviorFaults(faultStore, routes); err != nil {
		return 0, 0, err
	}

	reg.Register(openapi.New("openapi", routes, faultStore, metas, db))
	return len(specs), len(routes), nil
}

func logSpecWarnings(log *slog.Logger, specs []*openapi.LoadedSpec) {
	for _, spec := range specs {
		for _, w := range spec.Warnings {
			log.Warn("openapi spec uses unsupported construct", "spec", spec.Path, "warning", w)
		}
	}
}

// buildSpecRoutes resolves each spec's route prefix (erroring on collision
// with a hand-written provider or another spec) and builds its routes.
func buildSpecRoutes(reg *registry.Registry, specs []*openapi.LoadedSpec, persistDefault bool) ([][]openapi.Route, []openapi.SpecMeta, error) {
	reserved := make(map[string]string)
	for _, p := range reg.All() {
		reserved[p.Name] = p.Name
	}

	seenPrefix := make(map[string]string, len(specs))
	specRoutes := make([][]openapi.Route, 0, len(specs))
	metas := make([]openapi.SpecMeta, 0, len(specs))
	for _, spec := range specs {
		prefix, err := resolveSpecPrefix(spec, reserved, seenPrefix)
		if err != nil {
			return nil, nil, err
		}

		persist := persistDefault
		if spec.Persist != nil {
			persist = *spec.Persist
		}
		specRoutes = append(specRoutes, openapi.BuildRoutes(spec, prefix, persist))
		metas = append(metas, openapi.SpecMeta{
			Prefix:   prefix,
			Path:     spec.Path,
			Title:    spec.Title,
			Version:  spec.Version,
			Checksum: spec.Checksum,
		})
	}
	return specRoutes, metas, nil
}

// resolveSpecPrefix computes spec's route prefix and records it in
// seenPrefix, erroring if it collides with a reserved provider name or an
// earlier spec's prefix.
func resolveSpecPrefix(spec *openapi.LoadedSpec, reserved, seenPrefix map[string]string) (string, error) {
	prefix := specPrefix(spec)
	if owner, ok := reserved[prefix]; ok {
		return "", fmt.Errorf("openapi: spec %q resolves to prefix %q, which collides with provider %q — set x-mimic-name to disambiguate", spec.Path, prefix, owner)
	}
	if other, ok := seenPrefix[prefix]; ok {
		return "", fmt.Errorf("openapi: prefix %q used by both %q and %q — set x-mimic-name to disambiguate", prefix, other, spec.Path)
	}
	seenPrefix[prefix] = spec.Path
	return prefix, nil
}

// seedBehaviorFaults pre-populates fault_config with every route's
// x-mimic-behavior declaration, so RequestFaultHook applies spec-declared
// behavior through the same path as any admin-configured fault. Spec-seeded
// faults for the "openapi" provider are wiped and reseeded fresh on every
// boot (specs are re-parsed every boot too) — user-created faults for other
// providers are untouched. Known limitation: user-created faults against the
// "openapi" provider via PUT /admin/faults don't currently survive a
// restart alongside spec-seeded ones.
func seedBehaviorFaults(faultStore *admin.Store, routes []openapi.Route) error {
	if err := faultStore.DeleteFaultsByProvider("openapi"); err != nil {
		return fmt.Errorf("openapi: clear previously seeded behavior faults: %w", err)
	}
	for _, route := range routes {
		if route.Behavior == nil {
			continue
		}
		input, err := route.Behavior.ToCreateFaultInput("openapi", route.Path)
		if err != nil {
			return fmt.Errorf("openapi: invalid x-mimic-behavior on %s %s: %w", route.Method, route.Path, err)
		}
		if _, err := faultStore.CreateFault(input); err != nil {
			return fmt.Errorf("openapi: seed x-mimic-behavior fault for %s %s: %w", route.Method, route.Path, err)
		}
	}
	return nil
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// specPrefix derives a route prefix from a spec's x-mimic-name override (if
// set), falling back to its slugified title, falling back to its filename
// when no title is set either.
func specPrefix(spec *openapi.LoadedSpec) string {
	name := spec.MimicName
	if name == "" {
		name = spec.Title
	}
	if name == "" {
		base := filepath.Base(spec.Path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	slug := nonSlugChars.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(slug, "-")
}
