package main

import (
	"context"
	"fmt"
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
		fmt.Fprintf(os.Stderr, "startup config error: %v\n", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	listenAddr := ":" + cfg.Port
	logger.Info("startup begin",
		"app_env", cfg.AppEnv,
		"port", cfg.Port,
		"listen_addr", listenAddr,
		"storage_backend", cfg.StorageBackend,
	)
	logger.Info("database connect attempt started")
	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connect failed", "error", err.Error())
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("database connect succeeded")

	var st storage.Store
	if cfg.StorageBackend == "gcs" {
		logger.Info("storage initialization started", "backend", "gcs")
		gcsStore, err := storage.NewGCSStore(context.Background(), cfg.GCSBucket)
		if err != nil {
			logger.Error("storage initialization failed", "backend", "gcs", "error", err.Error())
			os.Exit(1)
		}
		defer gcsStore.Close()
		st = gcsStore
	} else {
		logger.Info("storage initialization started", "backend", "local")
		st = storage.NewLocalStore(cfg.StorageLocalDir, cfg.MediaPublicBaseURL)
	}
	logger.Info("storage initialization succeeded", "backend", cfg.StorageBackend)

	router := apphttp.NewRouterWithStore(cfg, logger, pool, st)
	srv := &http.Server{Addr: listenAddr, Handler: router, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("server starting", "listen_addr", listenAddr)
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
