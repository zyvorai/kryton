package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
)

type Input struct {
	Provider  provider.Provider
	Catalog   *catalog.Catalog
	AuthMode  string
	Projects  []string
	DockurDir string
	Runtime   string // docker | podman
}

func Run(ctx context.Context, in Input) model.DoctorReport {
	report := model.DoctorReport{Provider: in.Provider.Name(), Findings: []model.DoctorFinding{}}
	add := func(f model.DoctorFinding) { report.Findings = append(report.Findings, f) }

	add(checkAuth(in.AuthMode, in.Provider.Name()))
	add(checkProjects(in.Projects))
	add(checkCatalog(in.Catalog))
	add(checkProviderHealth(ctx, in.Provider))

	switch in.Provider.Name() {
	case "dockur":
		add(checkBinary(firstNonEmpty(in.Runtime, "docker"), "Container runtime for dockur/windows lab VMs"))
		add(checkCompose(firstNonEmpty(in.Runtime, "docker")))
		add(checkKVM())
		add(checkDirWritable(firstNonEmpty(in.DockurDir, filepath.Join(os.TempDir(), "kryton-dockur"))))
	case "kubevirt":
		add(checkHint("kubevirt-images", "warn", "Confirm CDI DataSources exist for each catalog image ID", "See docs/DEPLOYMENT.md image contract"))
	case "demo":
		add(model.DoctorFinding{Check: "demo-provider", Status: "pass", Message: "In-memory demo provider is active (no real Windows guests)"})
	}

	report.Healthy = true
	for _, f := range report.Findings {
		if f.Status == "fail" {
			report.Healthy = false
			break
		}
	}
	return report
}

func checkAuth(mode, providerName string) model.DoctorFinding {
	switch mode {
	case "disabled":
		if providerName != "demo" {
			return model.DoctorFinding{Check: "auth", Status: "warn", Message: "Authentication is disabled on a non-demo provider", Hint: "Use KRYTON_AUTH_MODE=apikey (or proxy) for production"}
		}
		return model.DoctorFinding{Check: "auth", Status: "pass", Message: "Auth disabled (local demo mode)"}
	case "apikey", "proxy":
		return model.DoctorFinding{Check: "auth", Status: "pass", Message: fmt.Sprintf("Auth mode %s", mode)}
	default:
		return model.DoctorFinding{Check: "auth", Status: "fail", Message: fmt.Sprintf("Unknown auth mode %q", mode)}
	}
}

func checkProjects(projects []string) model.DoctorFinding {
	if len(projects) == 0 {
		return model.DoctorFinding{Check: "projects", Status: "fail", Message: "No projects configured", Hint: "Set KRYTON_PROJECTS"}
	}
	return model.DoctorFinding{Check: "projects", Status: "pass", Message: fmt.Sprintf("%d project(s): %s", len(projects), strings.Join(projects, ", "))}
}

func checkCatalog(cat *catalog.Catalog) model.DoctorFinding {
	if cat == nil || len(cat.List()) == 0 {
		return model.DoctorFinding{Check: "catalog", Status: "fail", Message: "Image catalog is empty"}
	}
	items := cat.List()
	dockur := 0
	for _, img := range items {
		if img.DockurVersion != "" {
			dockur++
		}
	}
	msg := fmt.Sprintf("%d image(s) loaded", len(items))
	if dockur > 0 {
		msg += fmt.Sprintf(" (%d with dockurVersion)", dockur)
	}
	return model.DoctorFinding{Check: "catalog", Status: "pass", Message: msg}
}

func checkProviderHealth(ctx context.Context, p provider.Provider) model.DoctorFinding {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.Health(cctx); err != nil {
		return model.DoctorFinding{Check: "provider-health", Status: "fail", Message: err.Error(), Hint: "Fix provider connectivity before creating machines"}
	}
	return model.DoctorFinding{Check: "provider-health", Status: "pass", Message: fmt.Sprintf("Provider %s is healthy", p.Name())}
}

func checkBinary(name, purpose string) model.DoctorFinding {
	path, err := exec.LookPath(name)
	if err != nil {
		return model.DoctorFinding{Check: "runtime", Status: "fail", Message: fmt.Sprintf("%s not found in PATH", name), Hint: purpose}
	}
	return model.DoctorFinding{Check: "runtime", Status: "pass", Message: fmt.Sprintf("%s found at %s", name, path)}
}

func checkCompose(runtime string) model.DoctorFinding {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runtime, "compose", "version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return model.DoctorFinding{Check: "compose", Status: "fail", Message: fmt.Sprintf("%s compose unavailable: %v", runtime, err), Hint: "Install Docker Compose v2 or podman-compose"}
	} else {
		line := strings.TrimSpace(string(out))
		if i := strings.IndexByte(line, '\n'); i > 0 {
			line = line[:i]
		}
		return model.DoctorFinding{Check: "compose", Status: "pass", Message: line}
	}
}

func checkKVM() model.DoctorFinding {
	if runtime.GOOS != "linux" {
		return model.DoctorFinding{Check: "kvm", Status: "warn", Message: fmt.Sprintf("Host OS is %s; dockur needs Linux KVM (or nested virt)", runtime.GOOS), Hint: "Run dockur provider on a Linux host with /dev/kvm"}
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return model.DoctorFinding{Check: "kvm", Status: "fail", Message: "/dev/kvm is missing", Hint: "Enable VT-x/AMD-V in firmware and load kvm_intel/kvm_amd"}
	}
	return model.DoctorFinding{Check: "kvm", Status: "pass", Message: "/dev/kvm is present"}
}

func checkDirWritable(dir string) model.DoctorFinding {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.DoctorFinding{Check: "data-dir", Status: "fail", Message: err.Error(), Hint: "Set KRYTON_DOCKUR_DATA_DIR to a writable path"}
	}
	probe := filepath.Join(dir, ".kryton-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return model.DoctorFinding{Check: "data-dir", Status: "fail", Message: err.Error()}
	}
	_ = os.Remove(probe)
	return model.DoctorFinding{Check: "data-dir", Status: "pass", Message: fmt.Sprintf("Writable data dir %s", dir)}
}

func checkHint(name, status, message, hint string) model.DoctorFinding {
	return model.DoctorFinding{Check: name, Status: status, Message: message, Hint: hint}
}

func firstNonEmpty(v, d string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return d
}
