package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"yalinli-api/internal/config"
	"yalinli-api/internal/db"
	apphttp "yalinli-api/internal/http"
	"yalinli-api/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	var st storage.Store
	if cfg.StorageBackend == "gcs" {
		gcsStore, err := storage.NewGCSStore(context.Background(), cfg.GCSBucket)
		if err != nil {
			panic(err)
		}
		defer gcsStore.Close()
		st = gcsStore
	} else {
		st = storage.NewLocalStore(cfg.StorageLocalDir, cfg.MediaPublicBaseURL)
	}

	router := apphttp.NewRouterWithStore(cfg, logger, pool, st)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
