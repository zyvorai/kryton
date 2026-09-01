// Copyright 2026 Kryton contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zyvorai/kryton/internal/api"
	"github.com/zyvorai/kryton/internal/atlas"
	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/config"
	"github.com/zyvorai/kryton/internal/demo"
	"github.com/zyvorai/kryton/internal/dockur"
	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/golden"
	"github.com/zyvorai/kryton/internal/images"
	"github.com/zyvorai/kryton/internal/jobs"
	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/kubevirt"
	"github.com/zyvorai/kryton/internal/metrics"
	"github.com/zyvorai/kryton/internal/provider"
	"github.com/zyvorai/kryton/internal/reconciler"
	"github.com/zyvorai/kryton/internal/settings"
	"github.com/zyvorai/kryton/internal/storage"
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
	storagePath := cfg.StorageConfigFile
	if storagePath == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			storagePath = filepath.Join(home, ".kryton", "storage.json")
		}
	}
	storageStore, err := storage.NewStore(storagePath, cfg.StorageClass)
	if err != nil {
		log.Error("storage config store failed", "error", err)
		os.Exit(2)
	}
	settingsPath := os.Getenv("KRYTON_SETTINGS_CONFIG_FILE")
	if settingsPath == "" {
		if home, _ := os.UserHomeDir(); home != "" {
			settingsPath = filepath.Join(home, ".kryton", "settings.json")
		}
	}
	settingsStore, err := settings.NewStore(settingsPath, settings.Runtime{
		DefaultProject:  cfg.DefaultProject,
		ImageNamespace:  cfg.ImageNamespace,
		EventWebhookURL: cfg.EventWebhookURL,
		Atlas: atlas.Config{
			Enabled: os.Getenv("KRYTON_ATLAS_URL") != "",
			BaseURL: strings.TrimSpace(os.Getenv("KRYTON_ATLAS_URL")),
			Token:   strings.TrimSpace(os.Getenv("KRYTON_ATLAS_TOKEN")),
			Product: firstNonEmpty(os.Getenv("KRYTON_ATLAS_PRODUCT"), "kryton"),
		},
	})
	if err != nil {
		log.Error("settings store failed", "error", err)
		os.Exit(2)
	}
	runtimeSettings := settingsStore.Get()
	effectiveDefault := firstNonEmpty(runtimeSettings.DefaultProject, cfg.DefaultProject)
	effectiveImageNS := firstNonEmpty(runtimeSettings.ImageNamespace, cfg.ImageNamespace)
	effectiveSC := storageStore.Get().StorageClass
	if effectiveSC == "" {
		effectiveSC = cfg.StorageClass
	}
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
		p = kubevirt.New(kubevirt.Config{Client: kc, NamespacePrefix: cfg.NamespacePrefix, ImageNamespace: effectiveImageNS, StorageClass: effectiveSC})
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
	if runtimeSettings.EventWebhookURL != "" {
		bus.SetWebhookURL(runtimeSettings.EventWebhookURL)
	}
	m := metrics.New()
	projectRoot := findProjectRoot()
	var goldenMgr *golden.Manager
	var storageSetup *storage.SetupManager
	if projectRoot != "" {
		goldenMgr, _ = golden.New(golden.Config{
			BaseDir:        cfg.GoldenDir,
			ScriptPath:     filepath.Join(projectRoot, "scripts", "build-golden-image.sh"),
			BootstrapPath:  filepath.Join(projectRoot, "scripts", "bootstrap-kubevirt-images.sh"),
			OEMDir:         filepath.Join(projectRoot, "deploy", "dockur", "oem"),
			PublicHost:     firstNonEmpty(cfg.Dockur.PublicHost, "127.0.0.1"),
			ImageNamespace: effectiveImageNS,
			Resolver:       cat,
		})
		if cfg.Provider == "kubevirt" {
			storageSetup, _ = storage.NewSetupManager(storage.SetupConfig{
				SnapshotsScript: filepath.Join(projectRoot, "scripts", "enable-kubevirt-snapshots.sh"),
				RookScript:      filepath.Join(projectRoot, "scripts", "enable-rook-ceph.sh"),
				OnComplete: func(sc string, setDefault bool) {
					if !setDefault || sc == "" {
						return
					}
					_ = storageStore.Save(storage.Config{StorageClass: sc})
					if kv, ok := p.(*kubevirt.Provider); ok {
						kv.SetStorageClass(sc)
					}
				},
			})
		}
	}
	h := api.New(api.Config{
		Provider: p, Catalog: cat, Events: bus, Auth: authn, Metrics: m, Web: web,
		Projects: cfg.Projects, DefaultProject: effectiveDefault, AuthMode: cfg.AuthMode,
		DockurDataDir: cfg.Dockur.DataDir, DockurRuntime: cfg.Dockur.Runtime,
		ImageNamespace: effectiveImageNS, NamespacePrefix: cfg.NamespacePrefix, StorageClass: effectiveSC,
		StorageStore: storageStore, StorageSetup: storageSetup, SettingsStore: settingsStore, KubeClient: kubeClient, Golden: goldenMgr,
		AllowInsecure: cfg.AllowInsecure, LabAutoAuth: cfg.LabAutoAuth, LabTokenFile: cfg.LabTokenFile,
		DefaultProjectEnv: cfg.DefaultProject, ImageNamespaceEnv: cfg.ImageNamespace,
		StorageConfigPath: storagePath, SettingsConfigPath: settingsPath,
		CORSOrigins: cfg.CORSOrigins,
		Jobs: &jobs.Service{
			Provider: p, Golden: goldenMgr, StorageSetup: storageSetup, Projects: cfg.Projects,
			DockurData: cfg.Dockur.DataDir, DockurRun: cfg.Dockur.Runtime,
		},
		Inventory: &images.Inventory{
			Provider: p.Name(), ImageNS: effectiveImageNS, KubeClient: kubeClient,
			Golden: goldenMgr, ProjectRoot: projectRoot,
		},
		Log: log,
	}).Handler()
	server := &http.Server{Addr: cfg.Addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	go reconciler.TTL{Provider: p, Projects: cfg.Projects, Events: bus, Log: log, Interval: cfg.ReconcileInterval}.Run(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Info("Kryton starting", "addr", cfg.Addr, "provider", p.Name(), "auth", cfg.AuthMode, "projects", cfg.Projects, "storageClass", effectiveSC)
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

func findProjectRoot() string {
	if v := os.Getenv("KRYTON_PROJECT_ROOT"); v != "" {
		if _, err := os.Stat(filepath.Join(v, "scripts", "enable-kubevirt-snapshots.sh")); err == nil {
			return filepath.Clean(v)
		}
	}
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".deployments", "kryton"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, dir, filepath.Join(dir, ".."), filepath.Join(dir, "..", ".."))
	}
	for _, c := range candidates {
		c = filepath.Clean(c)
		if _, err := os.Stat(filepath.Join(c, "scripts", "enable-kubevirt-snapshots.sh")); err == nil {
			return c
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
