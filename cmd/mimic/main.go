// Command mimic is the Mimic simulator entrypoint: it wires storage,
// providers, the admin control-plane, and the embedded dashboard onto a
// single HTTP server, and hands the server + job scheduler lifecycle to
// Lane for startup/health/graceful-shutdown orchestration.
package main

import (
	"context"
	"database/sql"
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

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func parseServeFlags(args []string) openAPIFlags {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var f openAPIFlags
	fs.StringVar(&f.dir, "spec-dir", "", "directory to recursively scan for OpenAPI specs")
	fs.Var(&f.globs, "spec", "glob pattern matching OpenAPI spec files (repeatable)")
	fs.StringVar(&f.conflict, "conflict", string(openapi.ConflictStrict), "route conflict mode across specs: strict|priority|merge")
	_ = fs.Parse(args)
	return f
}

func printUsage(w *os.File) {
	_, _ = fmt.Fprintln(w, "mimic — simulates third-party provider APIs for local development and testing")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  mimic                 run the server (providers/behavior configured via env vars)")
	_, _ = fmt.Fprintln(w, "  mimic serve [flags]   run the server, optionally mounting OpenAPI specs")
	_, _ = fmt.Fprintln(w, "  mimic -h | --help     show this help")
	_, _ = fmt.Fprintln(w, "  mimic -version        print the version")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "serve flags:")
	_, _ = fmt.Fprintln(w, "  -spec-dir <path>   directory to recursively scan for OpenAPI specs")
	_, _ = fmt.Fprintln(w, "  -spec <glob>       glob pattern matching OpenAPI spec files (repeatable)")
	_, _ = fmt.Fprintln(w, "  -conflict <mode>   route conflict mode across specs: strict|priority|merge (default strict)")
}

func main() {
	oa := parseArgs(os.Args[1:])
	run(oa)
}

// parseArgs dispatches the top-level CLI argument (if any). "-h"/"-version"
// and unknown arguments exit directly, matching the previous inline switch
// in main(). Only "serve" (or no argument) returns normally.
func parseArgs(args []string) openAPIFlags {
	if len(args) == 0 {
		return openAPIFlags{}
	}
	switch args[0] {
	case "serve":
		return parseServeFlags(args[1:])
	case "-h", "-help", "--help", "help":
		printUsage(os.Stdout)
		os.Exit(0)
	case "-version", "--version", "version":
		fmt.Printf("mimic %s (commit %s, built %s)\n", version, commit, date)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "mimic: unknown argument %q\n\n", args[0])
		printUsage(os.Stderr)
		os.Exit(2)
	}
	panic("unreachable")
}

// discoverSpecDir returns the "specs" directory when oa didn't explicitly
// set -spec-dir/-spec and a non-empty "specs" directory exists in the cwd.
func discoverSpecDir(oa openAPIFlags) string {
	if oa.dir != "" || len(oa.globs) > 0 {
		return oa.dir
	}
	info, err := os.Stat("specs")
	if err != nil || !info.IsDir() {
		return ""
	}
	found, err := openapi.Discover("specs", nil)
	if err != nil || len(found) == 0 {
		return ""
	}
	return "specs"
}

func registerOpenAPIIfConfigured(reg *registry.Registry, db *sql.DB, faultStore *admin.Store, oa openAPIFlags, specDir string, cfg config.Config, log *slog.Logger) {
	if specDir == "" && len(oa.globs) == 0 {
		return
	}
	specCount, routeCount, err := providers.RegisterOpenAPI(reg, db, faultStore, providers.OpenAPIOptions{
		SpecDir:        specDir,
		SpecGlobs:      oa.globs,
		ConflictMode:   openapi.ConflictMode(oa.conflict),
		PersistDefault: cfg.OpenAPIPersist,
	}, log)
	if err != nil {
		log.Error("failed to register openapi adapter", "error", err)
		os.Exit(1)
	}
	log.Info("openapi specs loaded", "specs", specCount, "routes", routeCount)
}

func run(oa openAPIFlags) {
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
	providers.RegisterAll(reg, db, faultStore, sched, cfg.ZenviaStatusDelay, cfg.BryScadWebhookURL)

	specDir := discoverSpecDir(oa)
	registerOpenAPIIfConfigured(reg, db, faultStore, oa, specDir, cfg, log)

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
