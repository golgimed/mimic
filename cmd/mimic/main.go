// Command mimic is the Mimic simulator entrypoint: it wires storage,
// providers, the admin control-plane, and the embedded dashboard onto a
// single HTTP server, and hands the server + job scheduler lifecycle to
// Lane for startup/health/graceful-shutdown orchestration.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rluders/lane"
	"github.com/rluders/lane/runners"

	"github.com/golgimed/mimic/internal/config"
	"github.com/golgimed/mimic/internal/core"
	"github.com/golgimed/mimic/internal/openapi"
	"github.com/golgimed/mimic/internal/providers"
	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/faults"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/storage"
)

// specFlags collects repeated -spec flags into a slice.
type specFlags []string

func (s *specFlags) String() string     { return strings.Join(*s, ",") }
func (s *specFlags) Set(v string) error { *s = append(*s, v); return nil }

// openAPIFlags holds the "serve" subcommand's spec-loading flags. Zero
// value means "no OpenAPI adapter" — the plain env-var boot path (no
// subcommand) never sets these.
type openAPIFlags struct {
	dir      string
	globs    specFlags
	conflict string
}

func parseServeFlags(args []string) openAPIFlags {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var f openAPIFlags
	fs.StringVar(&f.dir, "spec-dir", "", "directory to recursively scan for OpenAPI specs")
	fs.Var(&f.globs, "spec", "glob pattern matching OpenAPI spec files (repeatable)")
	fs.StringVar(&f.conflict, "conflict", string(openapi.ConflictStrict), "route conflict mode across specs: strict|merge|priority")
	_ = fs.Parse(args)
	return f
}

func main() {
	var oa openAPIFlags
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		oa = parseServeFlags(os.Args[2:])
	}

	_ = config.LoadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(1)
	}

	lane.RunHealthCheck(fmt.Sprintf(":%d", cfg.Port))

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	if err := storage.RunMigrations(db); err != nil {
		log.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	faults.SetDefaultDelay(cfg.DefaultDelay)

	reg := registry.New()
	faultStore := admin.NewStore(db)
	sched := scheduler.New(db)
	providers.RegisterAll(reg, db, faultStore, sched, cfg.ZenviaStatusDelay)

	specDir := oa.dir
	if specDir == "" && len(oa.globs) == 0 {
		if info, err := os.Stat("specs"); err == nil && info.IsDir() {
			specDir = "specs"
		}
	}

	if specDir != "" || len(oa.globs) > 0 {
		specCount, routeCount, err := providers.RegisterOpenAPI(reg, db, faultStore, specDir, oa.globs, openapi.ConflictMode(oa.conflict), cfg.OpenAPIPersist, log)
		if err != nil {
			log.Error("failed to register openapi adapter", "error", err)
			os.Exit(1)
		}
		log.Info("openapi specs loaded", "specs", specCount, "routes", routeCount)
	}

	l := lane.New(log, lane.WithShutdownTimeout(10*time.Second))

	mux := core.NewMux(reg, db, faultStore, cfg.EnabledProviders, l.Health())
	handler := core.WithCORS(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: handler,
	}

	l.AddRunner(runners.NewHTTPRunner("api", server, log))
	l.AddRunner(runners.NewSchedulerRunner("jobs", cfg.SchedulerInterval, sched.Tick, log))

	l.Health().SetReady(true)
	printBanner(cfg.Port)

	if err := l.Run(context.Background()); err != nil {
		log.Error("mimic exited with error", "error", err)
		os.Exit(1)
	}
}

func printBanner(port int) {
	color := isTTY(os.Stdout)
	purple := func(s string) string {
		if color {
			return "\x1b[35m" + s + "\x1b[0m"
		}
		return s
	}
	dim := func(s string) string {
		if color {
			return "\x1b[2m" + s + "\x1b[0m"
		}
		return s
	}

	fmt.Printf("%s  %s\n", purple("MIMIC"), dim("looks like the provider, bites different"))
	fmt.Println(dim(fmt.Sprintf("listening on http://localhost:%d", port)))
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
