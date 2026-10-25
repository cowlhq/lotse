// Package hub runs the hub: it opens the database and serves the API and
// the web interface.
package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"lotse.sh/internal/db"
	"lotse.sh/internal/ui"
	"lotse.sh/internal/version"
)

// Config is how the hub is run, from LOTSE_* variables.
type Config struct {
	// DataDir holds the SQLite database and whatever else the hub keeps
	// on disk.
	DataDir string
	// Listen is the address the hub serves on.
	Listen string
	// DatabaseURL names a Postgres database. Empty means SQLite in DataDir.
	DatabaseURL string
}

// ConfigFromEnv reads the configuration from the environment.
func ConfigFromEnv() Config {
	return Config{
		DataDir:     cmp.Or(os.Getenv("LOTSE_DATA_DIR"), "/var/lib/lotse"),
		Listen:      cmp.Or(os.Getenv("LOTSE_LISTEN"), ":8080"),
		DatabaseURL: os.Getenv("LOTSE_DATABASE_URL"),
	}
}

// Run serves the hub until ctx ends.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	d, err := db.Open(ctx, cmp.Or(cfg.DatabaseURL, filepath.Join(cfg.DataDir, "lotse.db")))
	if err != nil {
		return err
	}
	defer d.Close()

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           New(d, ui.Files()),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	log.Info("lotse hub", "version", version.Version, "listen", cfg.Listen, "database", d.Dialect().String())

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-errs; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// New returns the hub's handler: the API under /api/, the web interface
// from files everywhere else.
func New(d *db.DB, files fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		status, ok := http.StatusOK, true
		if err := d.Ping(r.Context()); err != nil {
			status, ok = http.StatusServiceUnavailable, false
		}
		writeJSON(w, status, map[string]any{"ok": ok, "version": version.Version})
	})
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/", ui.Handler(files))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
