package main

import (
	"context"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func runMetrics(ctx context.Context, registry *prometheus.Registry, logger *zap.Logger) error {
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = "localhost:8081"
	}
	mux := http.NewServeMux()
	attachProfiler(mux)
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	server := &http.Server{Addr: metricsAddr, Handler: mux}

	grp, ctx := errgroup.WithContext(ctx)
	grp.Go(func() error {
		logger.Info("ListenAndServe", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	grp.Go(func() error {
		<-ctx.Done()
		logger.Debug("Shutting down")
		return server.Close()
	})

	return grp.Wait()
}
