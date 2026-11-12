// Command server runs the Sinapis ATS: the HR dashboard, the public careers
// pages and the background screening workers, in one process.
//
//	go run ./cmd/server
//
// Configuration comes from environment variables — see .env.example.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed time-zone data so Africa/Nairobi works in minimal containers

	"github.com/fmuoria/CV-Review-agent/internal/config"
	"github.com/fmuoria/CV-Review-agent/internal/db"
	"github.com/fmuoria/CV-Review-agent/internal/extract"
	"github.com/fmuoria/CV-Review-agent/internal/googleauth"
	"github.com/fmuoria/CV-Review-agent/internal/intake"
	"github.com/fmuoria/CV-Review-agent/internal/llm"
	"github.com/fmuoria/CV-Review-agent/internal/profile"
	"github.com/fmuoria/CV-Review-agent/internal/scoring"
	"github.com/fmuoria/CV-Review-agent/internal/screening"
	"github.com/fmuoria/CV-Review-agent/internal/secure"
	"github.com/fmuoria/CV-Review-agent/internal/storage"
	"github.com/fmuoria/CV-Review-agent/internal/store"
	"github.com/fmuoria/CV-Review-agent/internal/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := db.Migrate(ctx, conn); err != nil {
		return err
	}
	st := store.New(conn)

	// Google credentials are only needed for Vertex AI and Cloud Storage.
	var tokens googleauth.TokenSource
	if cfg.LLMProvider == "vertex" || cfg.StorageBackend == "gcs" {
		if tokens, err = googleauth.FromConfig(cfg.ServiceAccountJSON, cfg.ServiceAccountFile, googleauth.CloudPlatformScope); err != nil {
			return err
		}
	}

	var ai llm.Client
	switch cfg.LLMProvider {
	case "demo":
		d := llm.NewDemo()
		d.Handle("profile", profile.DemoHandler)
		d.Handle("score", scoring.DemoHandler)
		// Demo mode can't read scanned documents; they are marked "needs manual review".
		d.Handle("transcribe", func(llm.Request) (string, error) { return "", nil })
		ai = d
		log.Warn("LLM_PROVIDER=demo: scores are keyword matches, not AI. Set LLM_PROVIDER=vertex for real screening.")
	default:
		if ai, err = llm.NewGemini(llm.GeminiConfig{Provider: cfg.LLMProvider, Model: cfg.LLMModel, Project: cfg.GCPProject,
			Location: cfg.GCPLocation, APIKey: cfg.GeminiAPIKey, Tokens: tokens}); err != nil {
			return err
		}
	}

	var files storage.Store = storage.Local{Dir: cfg.StorageDir}
	if cfg.StorageBackend == "gcs" {
		files = storage.GCS{Bucket: cfg.GCSBucket, Tokens: tokens}
	}

	screener := &screening.Service{
		Store: st, Files: files, Extractor: &extract.Extractor{AI: ai}, Scorer: scoring.New(ai), AI: ai,
		Log: log, Workers: cfg.ScreeningWorkers, Delay: cfg.ScreeningDelay,
	}
	in := &intake.Service{Store: st, Files: files, MaxBytes: cfg.MaxUploadMB << 20, OnSaved: screener.Wake}

	srv := &web.Server{Cfg: cfg, Store: st, Files: files, Intake: in, Screening: screener,
		Box: secure.New(cfg.Secret()), Log: log, DemoAI: cfg.LLMProvider == "demo",
		Now: func() time.Time { return time.Now().In(cfg.Location) }}
	handler, err := srv.Handler()
	if err != nil {
		return err
	}

	go screener.Run(ctx)
	go cleanupSessions(ctx, st, log)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       5 * time.Minute, // large bulk uploads on slow connections
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", httpServer.Addr, "base_url", cfg.BaseURL, "env", cfg.Env, "ai", ai.Model(), "storage", cfg.StorageBackend)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

func cleanupSessions(ctx context.Context, st *store.Store, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := st.DeleteExpiredSessions(ctx); err != nil {
				log.Warn("session cleanup", "err", err)
			}
		}
	}
}
