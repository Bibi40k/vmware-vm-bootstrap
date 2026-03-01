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
	"regexp"
	"strings"
	"sync"

	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/vcenter"
)

type ovfImportSpec struct {
	Name           string `json:"Name,omitempty"`
	NetworkMapping []struct {
		Name    string `json:"Name,omitempty"`
		Network string `json:"Network,omitempty"`
	} `json:"NetworkMapping,omitempty"`
}

type govcLibraryInfo struct {
	Library struct {
		ID   string `json:"ID"`
		Name string `json:"Name"`
	} `json:"Library"`
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
		"GOVC_DATACENTER="+cfg.Datacenter,
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

var talosLibraryItemSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
var talosLibraryItemLocks sync.Map

func talosLibraryItemName(version, schematicID string) string {
	v := strings.TrimPrefix(normalizeTalosVersion(version), "v")
	if v == "" {
		v = "latest"
	}
	s := strings.TrimSpace(schematicID)
	if len(s) > 12 {
		s = s[:12]
	}
	if s == "" {
		s = "default"
	}
	name := fmt.Sprintf("talos-%s-%s", v, s)
	return talosLibraryItemSanitizer.ReplaceAllString(name, "-")
}

func isGovcAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already_exists") ||
		strings.Contains(msg, "duplicate_item_name_unsupported_in_library")
}

func govcErrContains(err error, needle string) bool {
	if err == nil || strings.TrimSpace(needle) == "" {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), strings.ToLower(needle))
}

func resolveContentLibraryID(ctx context.Context, env []string, name string) (string, error) {
	out, err := runGovc(ctx, env, "library.info", "-json", name)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return "", fmt.Errorf("matches 0 items")
	}
	// govc versions may return either:
	// - object: {"Library": {...}}
	// - array:  [{...}]
	var obj govcLibraryInfo
	if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
		id := strings.TrimSpace(obj.Library.ID)
		if id != "" {
			return id, nil
		}
	}
	var arr []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
		if len(arr) == 1 && strings.TrimSpace(arr[0].ID) != "" {
			return strings.TrimSpace(arr[0].ID), nil
		}
		if len(arr) > 1 {
			return "", fmt.Errorf("content library name %q is ambiguous; set vcenter.content_library_id", name)
		}
	}
	return "", fmt.Errorf("matches 0 items")
}

func ensureTalosContentLibrary(ctx context.Context, env []string, cfg *VMConfig) (libraryTarget, libraryName string, err error) {
	if id := strings.TrimSpace(cfg.ContentLibraryID); id != "" {
		return id, id, nil
	}
	name := strings.TrimSpace(cfg.ContentLibrary)
	if name == "" {
		name = "talos-images"
	}
	if id, err := resolveContentLibraryID(ctx, env, name); err == nil {
		return id, name, nil
	}
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "matches 0 items") {
			// create below
		} else if strings.Contains(msg, "matches ") && strings.Contains(msg, " items") {
			return "", "", fmt.Errorf("content library name %q is ambiguous; set vcenter.content_library_id", name)
		} else {
			return "", "", fmt.Errorf("resolve content library %q: %w", name, err)
		}
	}
	if _, err := runGovc(ctx, env, "library.create", name); err != nil {
		return "", "", fmt.Errorf("ensure content library %q: %w", name, err)
	}
	id, err := resolveContentLibraryID(ctx, env, name)
	if err != nil {
		return "", "", fmt.Errorf("resolve content library %q after create: %w", name, err)
	}
	return id, name, nil
}

func talosLibraryItemExists(ctx context.Context, env []string, libraryTarget, itemName string) (bool, error) {
	itemPath := fmt.Sprintf("%s/%s", strings.TrimSpace(libraryTarget), strings.TrimSpace(itemName))
	out, err := runGovc(ctx, env, "library.info", "-json", itemPath)
	if err != nil {
		return false, err
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return false, nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err == nil && len(obj) > 0 {
		return true, nil
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil && len(arr) > 0 {
		return true, nil
	}
	return false, nil
}

func removeTalosLibraryItem(ctx context.Context, env []string, libraryTarget, itemName string) {
	_, _ = runGovc(ctx, env, "library.rm", fmt.Sprintf("%s/%s", libraryTarget, itemName))
}

func importTalosLibraryItem(ctx context.Context, env []string, libraryTarget, itemName, ovaURL string) error {
	// Preferred path: vCenter pulls directly (best cache locality).
	if _, err := runGovc(ctx, env, "library.import", "-pull", "-n", itemName, libraryTarget, ovaURL); err == nil {
		return nil
	}
	// Fallback path: client-side import (works around some vCenter pull protocol issues).
	removeTalosLibraryItem(ctx, env, libraryTarget, itemName)
	if _, err := runGovc(ctx, env, "library.import", "-n", itemName, libraryTarget, ovaURL); err != nil && !isGovcAlreadyExists(err) {
		return fmt.Errorf("library.import failed (pull and fallback): %w", err)
	}
	return nil
}

func ensureTalosLibraryItem(ctx context.Context, env []string, libraryTarget, itemName, ovaURL string) error {
	lockKey := fmt.Sprintf("%s/%s", strings.TrimSpace(libraryTarget), strings.TrimSpace(itemName))
	muAny, _ := talosLibraryItemLocks.LoadOrStore(lockKey, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	exists, err := talosLibraryItemExists(ctx, env, libraryTarget, itemName)
	if err != nil {
		return fmt.Errorf("library.info failed: %w", err)
	}
	if exists {
		return nil
	}
	return importTalosLibraryItem(ctx, env, libraryTarget, itemName, ovaURL)
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
	libraryTarget, libraryName, err := ensureTalosContentLibrary(ctx, env, cfg)
	if err != nil {
		return nil, err
	}
	itemName := talosLibraryItemName(version, schematicID)
	logger.Info("Talos content library target", "library", libraryName, "item", itemName)
	if err := ensureTalosLibraryItem(ctx, env, libraryTarget, itemName, ovaURL); err != nil {
		return nil, err
	}

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

	args := []string{"library.deploy", "-dc", cfg.Datacenter, "-ds", cfg.Datastore, "-options", specPath}
	if cfg.Folder != "" {
		args = append(args, "-folder", cfg.Folder)
	}
	if cfg.ResourcePool != "" {
		args = append(args, "-pool", cfg.ResourcePool)
	}
	args = append(args, fmt.Sprintf("%s/%s", libraryTarget, itemName), cfg.Name)
	if _, err := runGovc(ctx, env, args...); err != nil {
		// Recover once from broken library item state (e.g. partial failed import).
		if govcErrContains(err, "invalid_library_item") || govcErrContains(err, "not an OVF") {
			removeTalosLibraryItem(ctx, env, libraryTarget, itemName)
			if impErr := importTalosLibraryItem(ctx, env, libraryTarget, itemName, ovaURL); impErr == nil {
				if _, retryErr := runGovc(ctx, env, args...); retryErr == nil {
					goto deployed
				} else {
					return nil, fmt.Errorf("govc library.deploy failed after item reimport: %w", retryErr)
				}
			} else {
				return nil, fmt.Errorf("recover invalid library item failed: %w", impErr)
			}
		}
		return nil, fmt.Errorf("govc library.deploy failed: %w", err)
	}
deployed:

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
