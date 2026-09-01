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

	"github.com/cankonix/xpace/api/internal/httpapi"
	"github.com/cankonix/xpace/api/internal/platform"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := platform.ValidateRuntimeSecrets(); err != nil {
		logger.Error("runtime secret validation failed", "error", err)
		os.Exit(1)
	}
	database, err := platform.OpenPostgres(context.Background())
	if err != nil {
		logger.Error("postgres initialization failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	workerContext, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	httpapi.StartGovernanceRetentionWorker(workerContext, database, logger)
	httpapi.StartDataExportWorker(workerContext, database, logger)
	httpapi.StartEmailWorker(workerContext, database, logger)
	httpapi.StartMeetingDurationWorker(workerContext, database, logger)
	httpapi.StartIncidentEscalationWorker(workerContext, database, logger)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           httpapi.NewRouter(database, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errorsChannel := make(chan error, 1)
	go func() {
		logger.Info("xpace api started", "address", server.Addr)
		errorsChannel <- server.ListenAndServe()
	}()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	select {
	case signal := <-signalChannel:
		logger.Info("shutdown signal received", "signal", signal.String())
	case err := <-errorsChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("xpace api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}
	stopWorker()

	context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(context); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}
