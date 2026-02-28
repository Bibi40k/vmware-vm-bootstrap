package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/vcenter"
)

type ovfImportSpec struct {
	Name           string `json:"Name,omitempty"`
	NetworkMapping []struct {
		Name    string `json:"Name,omitempty"`
		Network string `json:"Network,omitempty"`
	} `json:"NetworkMapping,omitempty"`
}

func normalizeTalosVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

func talosOVAURL(version, schematicID string) string {
	return fmt.Sprintf("https://factory.talos.dev/image/%s/%s/vmware-amd64.ova",
		strings.TrimSpace(schematicID), normalizeTalosVersion(version))
}

func govcEnv(cfg *VMConfig) []string {
	url := cfg.VCenterHost
	if !strings.Contains(url, "://") {
		url = "https://" + url + "/sdk"
	}
	return append(os.Environ(),
		"GOVC_URL="+url,
		"GOVC_USERNAME="+cfg.VCenterUsername,
		"GOVC_PASSWORD="+cfg.VCenterPassword,
		fmt.Sprintf("GOVC_INSECURE=%t", cfg.VCenterInsecure),
	)
}

func runGovc(ctx context.Context, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "govc", args...)
	cmd.Env = env
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out.Bytes(), nil
}

// CreateTalosNodeFromOVA deploys a Talos VMware OVA and powers the VM on.
func CreateTalosNodeFromOVA(ctx context.Context, cfg *VMConfig, logger *slog.Logger) (*VM, error) {
	if logger == nil {
		logger = defaultLogger
	}
	cfg.SetDefaults()
	if cfg.Profile != "talos" {
		return nil, fmt.Errorf("CreateTalosNodeFromOVA requires Profile=talos")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if cfg.Datacenter == "" || cfg.Datastore == "" {
		return nil, fmt.Errorf("datacenter and datastore are required")
	}

	version := normalizeTalosVersion(cfg.EffectiveTalosVersion())
	if version == "" {
		return nil, fmt.Errorf("Profiles.Talos.Version is required")
	}

	schematicID := strings.TrimSpace(cfg.Profiles.Talos.SchematicID)
	if schematicID == "" {
		return nil, fmt.Errorf("Profiles.Talos.SchematicID is required")
	}
	ovaURL := talosOVAURL(version, schematicID)
	logger.Info("Deploying Talos OVA", "url", ovaURL, "version", version, "schematic_id", schematicID)

	env := govcEnv(cfg)

	specRaw, err := runGovc(ctx, env, "import.spec", ovaURL)
	if err != nil {
		return nil, fmt.Errorf("govc import.spec failed: %w", err)
	}
	var spec ovfImportSpec
	if err := json.Unmarshal(specRaw, &spec); err != nil {
		return nil, fmt.Errorf("parse import spec: %w", err)
	}
	spec.Name = cfg.Name
	if cfg.NetworkName != "" {
		for i := range spec.NetworkMapping {
			spec.NetworkMapping[i].Network = cfg.NetworkName
		}
	}

	if err := os.MkdirAll("tmp", 0o755); err != nil {
		return nil, err
	}
	specPath := filepath.Join("tmp", fmt.Sprintf("ova-import-%s.json", cfg.Name))
	specBytes, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal import spec: %w", err)
	}
	if err := os.WriteFile(specPath, specBytes, 0o600); err != nil {
		return nil, fmt.Errorf("write import spec: %w", err)
	}
	defer func() { _ = os.Remove(specPath) }()

	args := []string{"import.ova", "-dc", cfg.Datacenter, "-ds", cfg.Datastore, "-name", cfg.Name, "-options", specPath}
	if cfg.Folder != "" {
		args = append(args, "-folder", cfg.Folder)
	}
	if cfg.ResourcePool != "" {
		args = append(args, "-pool", cfg.ResourcePool)
	}
	args = append(args, ovaURL)
	if _, err := runGovc(ctx, env, args...); err != nil {
		return nil, fmt.Errorf("govc import.ova failed: %w", err)
	}

	if _, err := runGovc(ctx, env, "vm.power", "-on", cfg.Name); err != nil {
		return nil, fmt.Errorf("failed to power on Talos VM: %w", err)
	}

	vc, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     cfg.VCenterHost,
		Port:     cfg.VCenterPort,
		Username: cfg.VCenterUsername,
		Password: cfg.VCenterPassword,
		Insecure: cfg.VCenterInsecure,
	})
	if err != nil {
		return nil, fmt.Errorf("vCenter connection failed after OVA deploy: %w", err)
	}
	defer func() { _ = vc.Disconnect() }()

	vmObj, err := vc.FindVM(cfg.Datacenter, cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to locate created Talos VM: %w", err)
	}
	if vmObj == nil {
		return nil, fmt.Errorf("created Talos VM not found: %s", cfg.Name)
	}

	managed := vmObj.Reference()
	return &VM{
		Name:            cfg.Name,
		IPAddress:       cfg.IPAddress,
		ManagedObject:   managed,
		SSHReady:        false,
		Hostname:        cfg.Name,
		VCenterHost:     cfg.VCenterHost,
		VCenterPort:     cfg.VCenterPort,
		VCenterUser:     cfg.VCenterUsername,
		VCenterPass:     cfg.VCenterPassword,
		VCenterInsecure: cfg.VCenterInsecure,
	}, nil
}
