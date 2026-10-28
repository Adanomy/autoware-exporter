package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/config"
	"github.com/Adanomy/autoware-exporter/internal/exporter"
	"github.com/Adanomy/autoware-exporter/internal/monitor"
	"github.com/Adanomy/autoware-exporter/internal/ros2"
	"github.com/Adanomy/autoware-exporter/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var version = "dev"
var revision = "unknown"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "autoware-exporter: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := config.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		config.Usage(stdout)
		return nil
	}
	if err != nil {
		return err
	}
	if cfg.Version {
		_, err := fmt.Fprintf(stdout, "autoware-exporter %s revision=%s go=%s native_ros2=%t\n", version, revision, runtime.Version(), ros2.Available())
		return err
	}
	sources, err := selectedSources(cfg.Collectors, cfg.MaxAge)
	if err != nil {
		return err
	}
	if !ros2.Available() {
		return fmt.Errorf("native ROS 2 support is unavailable: build on Linux with CGO_ENABLED=1 and -tags ros2 in the matching ROS/Autoware environment")
	}
	level := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}[cfg.LogLevel]
	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler = slog.NewTextHandler(stderr, options)
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(stderr, options)
	}
	logger := slog.New(handler)
	vehicles := make([]ros2.Config, len(cfg.Vehicles))
	for i, vehicle := range cfg.Vehicles {
		vehicles[i] = ros2.Config{Name: vehicle.Name, DomainID: vehicle.DomainID, Namespace: vehicle.Namespace}
	}
	manager, err := monitor.New(vehicles, sources, monitor.Options{PollInterval: cfg.PollInterval, RetryInterval: cfg.RetryInterval}, logger)
	if err != nil {
		return err
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter.NewCollector(manager, version, revision), collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	server := &http.Server{
		Handler: exporter.NewHandler(registry), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second,
		IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var workers sync.WaitGroup
	workers.Go(func() { manager.Run(ctx) })
	served := make(chan error, 1)
	workers.Go(func() { served <- server.Serve(listener) })
	logger.Info("Autoware exporter started", "listen_address", listener.Addr(), "vehicles", len(vehicles), "sources_per_vehicle", len(sources), "version", version)
	select {
	case <-ctx.Done():
	case err = <-served:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
		err = errors.Join(err, fmt.Errorf("shut down HTTP server: %w", shutdownErr))
	}
	workers.Wait()
	logger.Info("Autoware exporter stopped")
	return err
}

func selectedSources(groups []string, maxAge time.Duration) ([]telemetry.Source, error) {
	for _, group := range groups {
		if !slices.Contains(telemetry.Groups(), group) {
			return nil, fmt.Errorf("unknown collector group %q; available: %v", group, telemetry.Groups())
		}
	}
	var sources []telemetry.Source
	for _, source := range telemetry.Sources() {
		// Remaining-route estimates require the route state even without the state group.
		dependency := source.Key == "route_state" && slices.Contains(groups, "routing")
		if len(groups) > 0 && !slices.Contains(groups, source.Group) && !dependency {
			continue
		}
		if !source.Latched {
			source.MaxAge = maxAge
		}
		sources = append(sources, source)
	}
	return sources, nil
}
