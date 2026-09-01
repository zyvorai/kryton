package golden

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/kryton/internal/model"
)

type VersionResolver interface {
	DockurVersion(imageID string) (string, bool)
}

type Manager struct {
	mu               sync.Mutex
	running          map[string]struct{}
	bootstrapping    map[string]struct{}
	baseDir          string
	scriptPath       string
	bootstrapPath    string
	oemDir           string
	publicHost       string
	imageNamespace   string
	versionMap       map[string]string
	resolver         VersionResolver
}

type Config struct {
	BaseDir          string
	ScriptPath       string
	BootstrapPath    string
	OEMDir           string
	PublicHost       string
	ImageNamespace   string
	Resolver         VersionResolver
}

func New(cfg Config) (*Manager, error) {
	base := strings.TrimSpace(cfg.BaseDir)
	if base == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		base = filepath.Join(home, ".kryton", "golden")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	host := strings.TrimSpace(cfg.PublicHost)
	if host == "" {
		host = "127.0.0.1"
	}
	ns := strings.TrimSpace(cfg.ImageNamespace)
	if ns == "" {
		ns = "kryton-images"
	}
	return &Manager{
		running:        map[string]struct{}{},
		bootstrapping:  map[string]struct{}{},
		baseDir:        base,
		scriptPath:     cfg.ScriptPath,
		bootstrapPath:  cfg.BootstrapPath,
		oemDir:         cfg.OEMDir,
		publicHost:     host,
		imageNamespace: ns,
		resolver:       cfg.Resolver,
		versionMap:     defaultVersions(),
	}, nil
}

var (
	ErrNotFound          = fmt.Errorf("golden build not found")
	ErrNotReady          = fmt.Errorf("golden image is not ready to publish")
	ErrBootstrapRunning  = fmt.Errorf("CDI bootstrap already running for this build")
	ErrBootstrapDisabled = fmt.Errorf("CDI bootstrap script is not configured")
)

func defaultVersions() map[string]string {
	return map[string]string{
		"windows-11-enterprise": "11e",
		"windows-server-2025": "2025",
		"windows-server-2022": "2022",
		"windows-11-pro":      "11",
		"windows-10-pro":      "10",
		"windows-server-2019": "2019",
	}
}

func (m *Manager) BaseDir() string { return m.baseDir }

func (m *Manager) Available() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found")
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return fmt.Errorf("/dev/kvm not available")
	}
	if m.scriptPath == "" {
		return fmt.Errorf("golden build script not configured")
	}
	if _, err := os.Stat(m.scriptPath); err != nil {
		return fmt.Errorf("golden build script missing: %w", err)
	}
	return nil
}

func (m *Manager) List() ([]model.GoldenBuild, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}
	var out []model.GoldenBuild
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := m.readStatus(filepath.Join(m.baseDir, e.Name(), "status.json"))
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	sortBuilds(out)
	return out, nil
}

func (m *Manager) Get(id string) (*model.GoldenBuild, error) {
	b, err := m.readStatus(filepath.Join(m.baseDir, id, "status.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}

func (m *Manager) Bootstrap(id string) (*model.GoldenBuild, error) {
	if strings.TrimSpace(m.bootstrapPath) == "" {
		return nil, ErrBootstrapDisabled
	}
	if _, err := os.Stat(m.bootstrapPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBootstrapDisabled, err)
	}
	b, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if b.State != model.GoldenReady {
		return nil, ErrNotReady
	}
	out := strings.TrimSpace(b.OutputPath)
	if out == "" {
		return nil, ErrNotReady
	}
	if _, err := os.Stat(out); err != nil {
		return nil, fmt.Errorf("golden artifact missing: %w", err)
	}
	m.mu.Lock()
	if _, ok := m.bootstrapping[id]; ok {
		m.mu.Unlock()
		return nil, ErrBootstrapRunning
	}
	m.bootstrapping[id] = struct{}{}
	m.mu.Unlock()

	b.BootstrapState = "running"
	b.BootstrapMessage = "Publishing qcow2 to CDI DataSource " + m.imageNamespace + "/" + b.ImageID
	workdir := filepath.Join(m.baseDir, id)
	if err := m.writeStatus(workdir, *b); err != nil {
		m.mu.Lock()
		delete(m.bootstrapping, id)
		m.mu.Unlock()
		return nil, err
	}
	go m.runBootstrap(id, b.ImageID, out, workdir)
	return m.Get(id)
}

func (m *Manager) Start(ctx context.Context, req model.GoldenStartRequest) (*model.GoldenBuild, error) {
	if err := m.Available(); err != nil {
		return nil, err
	}
	version, imageID, err := m.resolve(req)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%s-%d", version, time.Now().Unix())
	workdir := filepath.Join(m.baseDir, id)

	m.mu.Lock()
	if _, ok := m.running[id]; ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("golden build already running")
	}
	m.running[id] = struct{}{}
	m.mu.Unlock()

	now := time.Now().UTC()
	build := model.GoldenBuild{
		ID: id, Version: version, ImageID: imageID,
		State: model.GoldenStarting, Phase: "prepare",
		ProgressPercent: 5, Message: "Starting dockur/windows builder",
		StartedAt: now, UpdatedAt: now,
	}
	if err := m.writeStatus(workdir, build); err != nil {
		return nil, err
	}

	go m.runBuild(id, version, imageID, workdir, req.Auto)

	b, err := m.Get(id)
	if err != nil {
		return &build, nil
	}
	return b, nil
}

func (m *Manager) resolve(req model.GoldenStartRequest) (version, imageID string, err error) {
	imageID = strings.TrimSpace(req.ImageID)
	if imageID == "" {
		imageID = "windows-11-enterprise"
	}
	version = strings.TrimSpace(req.Version)
	if version == "" {
		if m.resolver != nil {
			if v, ok := m.resolver.DockurVersion(imageID); ok && v != "" {
				version = v
			}
		}
		if version == "" {
			var ok bool
			version, ok = m.versionMap[imageID]
			if !ok {
				return "", "", fmt.Errorf("unknown image %q (set version explicitly)", imageID)
			}
		}
	}
	return version, imageID, nil
}

func (m *Manager) runBuild(id, version, imageID, workdir string, auto bool) {
	defer func() {
		m.mu.Lock()
		delete(m.running, id)
		m.mu.Unlock()
	}()

	args := []string{m.scriptPath, "--build-id", id, "--workdir", workdir, "--version", version, "--image-id", imageID, "--host", m.publicHost}
	if auto {
		args = append(args, "--auto")
	}
	if m.oemDir != "" {
		args = append(args, "--oem", m.oemDir)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		b, _ := m.Get(id)
		if b != nil {
			b.State = model.GoldenFailed
			b.Error = strings.TrimSpace(string(out))
			if b.Error == "" {
				b.Error = err.Error()
			}
			b.Message = "Golden image build failed"
			b.UpdatedAt = time.Now().UTC()
			_ = m.writeStatus(workdir, *b)
		}
	}
}

func (m *Manager) runBootstrap(id, imageID, artifact, workdir string) {
	defer func() {
		m.mu.Lock()
		delete(m.bootstrapping, id)
		m.mu.Unlock()
	}()
	cmd := exec.Command(m.bootstrapPath, "--image", artifact, "--id", imageID)
	cmd.Env = append(os.Environ(), "KRYTON_IMAGE_NAMESPACE="+m.imageNamespace, "KRYTON_IMAGE_ID="+imageID, "KRYTON_WINDOWS_IMAGE="+artifact)
	out, err := cmd.CombinedOutput()
	b, _ := m.Get(id)
	if b == nil {
		return
	}
	if err != nil {
		b.BootstrapState = "failed"
		b.BootstrapMessage = strings.TrimSpace(string(out))
		if b.BootstrapMessage == "" {
			b.BootstrapMessage = err.Error()
		}
	} else {
		b.BootstrapState = "ready"
		b.DataSource = m.imageNamespace + "/" + imageID
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = "DataSource ready: " + b.DataSource
		}
		b.BootstrapMessage = msg
	}
	b.UpdatedAt = time.Now().UTC()
	_ = m.writeStatus(workdir, *b)
}

func (m *Manager) readStatus(path string) (model.GoldenBuild, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.GoldenBuild{}, err
	}
	var build model.GoldenBuild
	if err := json.Unmarshal(b, &build); err != nil {
		return model.GoldenBuild{}, err
	}
	return build, nil
}

func (m *Manager) writeStatus(workdir string, build model.GoldenBuild) error {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}
	build.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(build, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workdir, "status.json"), b, 0o644)
}

func sortBuilds(items []model.GoldenBuild) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].UpdatedAt.After(items[i].UpdatedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}