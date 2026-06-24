package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"effective-mobile/internal/config"
	"effective-mobile/internal/handler"
	"effective-mobile/internal/logger"
	mw "effective-mobile/internal/middleware"
	"effective-mobile/internal/repository"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed docs/swagger.yaml
var swaggerYAML []byte

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	lg := logger.New(cfg.Log.Level)
	lg.Info("starting service")

	rootCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := connectDB(rootCtx, cfg.Database.DSN(), lg)
	if err != nil {
		lg.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		lg.Error("init migrations FS", "err", err)
		os.Exit(1)
	}
	if err := repository.RunMigrations(rootCtx, pool, migrations, lg); err != nil {
		lg.Error("run migrations", "err", err)
		os.Exit(1)
	}

	repo := repository.New(pool)
	h := handler.New(repo, lg)

	r := chi.NewRouter()
	r.Use(mw.Recover(lg))
	r.Use(mw.Logger(lg))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/swagger.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(swaggerYAML)
	})
	r.Get("/swagger", swaggerUIHandler)
	r.Mount("/", h.Routes())

	srv := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      r,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	go func() {
		lg.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			lg.Error("server error", "err", err)
			cancel()
		}
	}()

	<-rootCtx.Done()
	lg.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("shutdown", "err", err)
	}
	lg.Info("stopped")
}

// connectDB retries the initial connection so that we don't race the
// PostgreSQL container during `docker compose up`.
func connectDB(ctx context.Context, dsn string, lg *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	const maxAttempts = 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = pool.Ping(ctx); err == nil {
			lg.Info("connected to database")
			return pool, nil
		}
		lg.Info("waiting for database", "attempt", attempt, "err", err.Error())
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	pool.Close()
	return nil, err
}

const swaggerHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Subscriptions API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => SwaggerUIBundle({ url: '/swagger.yaml', dom_id: '#swagger-ui' });
  </script>
</body>
</html>`

func swaggerUIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}
