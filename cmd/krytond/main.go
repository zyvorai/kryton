package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zyvorai/kryton/internal/api"
	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/config"
	"github.com/zyvorai/kryton/internal/demo"
	"github.com/zyvorai/kryton/internal/dockur"
	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/kubevirt"
	"github.com/zyvorai/kryton/internal/metrics"
	"github.com/zyvorai/kryton/internal/provider"
	"github.com/zyvorai/kryton/internal/reconciler"
)

//go:embed web/*
var assets embed.FS

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(2)
	}
	cat, err := catalog.Load(cfg.ImagesFile)
	if err != nil {
		log.Error("image catalog failed", "error", err)
		os.Exit(2)
	}
	authn, err := auth.New(auth.Config{Mode: cfg.AuthMode, APIKeysFile: cfg.APIKeysFile, TrustProxy: cfg.TrustProxy, ProxySecretFile: cfg.ProxySecretFile})
	if err != nil {
		log.Error("authentication configuration failed", "error", err)
		os.Exit(2)
	}

	var p provider.Provider
	var kubeClient *kubeapi.Client
	switch cfg.Provider {
	case "kubevirt":
		kc, err := kubeapi.New(kubeapi.Config{
			Endpoint: cfg.Kubernetes.Endpoint, BearerToken: cfg.Kubernetes.BearerToken,
			TokenFile: cfg.Kubernetes.TokenFile, CAFile: cfg.Kubernetes.CAFile,
			ClientCertFile: cfg.Kubernetes.ClientCertFile, ClientKeyFile: cfg.Kubernetes.ClientKeyFile,
			InsecureSkipVerify: cfg.Kubernetes.InsecureSkipVerify,
		})
		if err != nil {
			log.Error("Kubernetes client failed", "error", err)
			os.Exit(2)
		}
		kubeClient = kc
		p = kubevirt.New(kubevirt.Config{Client: kc, NamespacePrefix: cfg.NamespacePrefix, ImageNamespace: cfg.ImageNamespace})
		if err := kubevirt.EnsureNamespaces(context.Background(), kc, cfg.NamespacePrefix, cfg.Projects); err != nil {
			log.Warn("project namespace bootstrap failed", "error", err)
		}
	case "dockur":
		dp, err := dockur.New(dockur.Config{
			Runtime: cfg.Dockur.Runtime, DataDir: cfg.Dockur.DataDir, PublicHost: cfg.Dockur.PublicHost,
			HTTPBase: cfg.Dockur.HTTPBase, RDPBase: cfg.Dockur.RDPBase, Catalog: cat,
		})
		if err != nil {
			log.Error("dockur provider failed", "error", err)
			os.Exit(2)
		}
		p = dp
	default:
		p = demo.New()
	}

	web, err := fs.Sub(assets, "web")
	if err != nil {
		log.Error("embedded UI unavailable", "error", err)
		os.Exit(2)
	}
	bus, err := events.New(500, cfg.EventWebhookURL, cfg.EventWebhookSecret, cfg.EventsFile, log)
	if err != nil {
		log.Error("event bus failed", "error", err)
		os.Exit(2)
	}
	m := metrics.New()
	h := api.New(api.Config{
		Provider: p, Catalog: cat, Events: bus, Auth: authn, Metrics: m, Web: web,
		Projects: cfg.Projects, DefaultProject: cfg.DefaultProject, AuthMode: cfg.AuthMode,
		DockurDataDir: cfg.Dockur.DataDir, DockurRuntime: cfg.Dockur.Runtime,
		ImageNamespace: cfg.ImageNamespace, NamespacePrefix: cfg.NamespacePrefix, KubeClient: kubeClient, Log: log,
	}).Handler()
	server := &http.Server{Addr: cfg.Addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	go reconciler.TTL{Provider: p, Projects: cfg.Projects, Events: bus, Log: log, Interval: cfg.ReconcileInterval}.Run(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Info("Kryton starting", "addr", cfg.Addr, "provider", p.Name(), "auth", cfg.AuthMode, "projects", cfg.Projects)
		if cfg.TLS.CertFile != "" {
			if cfg.TLS.ClientCAFile != "" {
				pem, err := os.ReadFile(cfg.TLS.ClientCAFile)
				if err != nil {
					errCh <- err
					return
				}
				pool := x509.NewCertPool()
				if !pool.AppendCertsFromPEM(pem) {
					errCh <- errors.New("client CA file contains no certificates")
					return
				}
				server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
			}
			errCh <- server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
			return
		}
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		_ = server.Close()
	}
	log.Info("Kryton stopped")
}
