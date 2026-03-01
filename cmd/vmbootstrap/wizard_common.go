package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/vcenter"
	"gopkg.in/yaml.v3"
)

// WizardSession standardizes draft lifecycle for interactive wizards.
type WizardSession struct {
	TargetPath string
	DraftPath  string
	State      any
	IsEmpty    func() bool

	stopFn func()
}

func NewWizardSession(targetPath, draftPath string, state any, isEmpty func() bool) *WizardSession {
	return &WizardSession{
		TargetPath: targetPath,
		DraftPath:  draftPath,
		State:      state,
		IsEmpty:    isEmpty,
	}
}

func (w *WizardSession) LoadDraft() (bool, error) {
	if w == nil {
		return false, nil
	}
	return loadDraftYAML(w.DraftPath, w.State)
}

func (w *WizardSession) Start() {
	if w == nil {
		return
	}
	w.stopFn = startYAMLDraftHandler(w.TargetPath, w.DraftPath, w.State, w.IsEmpty)
}

func (w *WizardSession) Stop() {
	if w == nil || w.stopFn == nil {
		return
	}
	w.stopFn()
	w.stopFn = nil
}

func (w *WizardSession) Finalize() error {
	if w == nil {
		return nil
	}
	return cleanupDrafts(w.TargetPath)
}

type WizardStep struct {
	Name string
	Run  func() error
}

func runWizardSteps(steps []WizardStep) error {
	total := len(steps)
	for i, step := range steps {
		if strings.TrimSpace(step.Name) != "" {
			fmt.Printf("[%d/%d] %s\n", i+1, total, step.Name)
		}
		if step.Run == nil {
			continue
		}
		if err := step.Run(); err != nil {
			return err
		}
		if i < total-1 {
			fmt.Println()
		}
	}
	return nil
}

// VCenterCatalog is a reusable snapshot of vCenter pick-list resources.
type VCenterCatalog struct {
	Datastores []vcenter.DatastoreInfo
	Networks   []vcenter.NetworkInfo
	Folders    []vcenter.FolderInfo
	Pools      []vcenter.ResourcePoolInfo
}

func fetchVCenterCatalog(vcCfg *vcenterFileConfig, timeout time.Duration) (*VCenterCatalog, error) {
	if vcCfg == nil {
		return nil, fmt.Errorf("vcenter config is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	vclient, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     vcCfg.VCenter.Host,
		Username: vcCfg.VCenter.Username,
		Password: vcCfg.VCenter.Password,
		Port:     vcCfg.VCenter.Port,
		Insecure: vcCfg.VCenter.Insecure,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = vclient.Disconnect() }()

	ds, _ := vclient.ListDatastores(vcCfg.VCenter.Datacenter)
	nets, _ := vclient.ListNetworks(vcCfg.VCenter.Datacenter)
	fl, _ := vclient.ListFolders(vcCfg.VCenter.Datacenter)
	pl, _ := vclient.ListResourcePools(vcCfg.VCenter.Datacenter)

	return &VCenterCatalog{
		Datastores: ds,
		Networks:   nets,
		Folders:    fl,
		Pools:      pl,
	}, nil
}

func pickDatastoreFromCatalog(cat *VCenterCatalog, current string) string {
	return pickDatastoreFromCatalogWithPrompt(cat, current, "Default datastore", "Default datastore:")
}

func pickNetworkFromCatalog(cat *VCenterCatalog, current string) string {
	return pickNetworkFromCatalogWithPrompt(cat, current, "Default network", "Default network:")
}

func pickFolderFromCatalog(cat *VCenterCatalog, current string) string {
	return pickFolderFromCatalogWithPrompt(cat, current, "Default folder", "Default folder:")
}

func pickResourcePoolFromCatalog(cat *VCenterCatalog, current string) string {
	return pickResourcePoolFromCatalogWithPrompt(cat, current, "Default resource pool", "Default resource pool:")
}

func pickDatastoreFromCatalogWithPrompt(cat *VCenterCatalog, current, manualPrompt, listHeader string) string {
	if cat == nil || len(cat.Datastores) == 0 {
		return readLine(manualPrompt, current)
	}
	if strings.TrimSpace(listHeader) != "" {
		fmt.Printf("  %s\n", listHeader)
	}
	return selectISODatastore(cat.Datastores, current)
}

func pickNetworkFromCatalogWithPrompt(cat *VCenterCatalog, current, manualPrompt, listLabel string) string {
	if cat == nil || len(cat.Networks) == 0 {
		return readLine(manualPrompt, current)
	}
	return interactiveSelect(vcenterNetworkLeafNames(cat.Networks), current, listLabel)
}

func pickFolderFromCatalogWithPrompt(cat *VCenterCatalog, current, manualPrompt, listLabel string) string {
	if cat == nil || len(cat.Folders) == 0 {
		return readLine(manualPrompt, current)
	}
	return selectFolder(cat.Folders, current, listLabel)
}

func pickResourcePoolFromCatalogWithPrompt(cat *VCenterCatalog, current, manualPrompt, listLabel string) string {
	if cat == nil || len(cat.Pools) == 0 {
		return readLine(manualPrompt, current)
	}
	return selectResourcePool(cat.Pools, current, listLabel)
}

func vcenterNetworkLeafNames(networks []vcenter.NetworkInfo) []string {
	out := make([]string, 0, len(networks))
	for _, n := range networks {
		parts := strings.Split(n.Name, "/")
		out = append(out, parts[len(parts)-1])
	}
	return out
}

func catalogIfReady(cat *VCenterCatalog, err error) *VCenterCatalog {
	if err != nil {
		return nil
	}
	return cat
}

// loadDraftYAML loads draft YAML into out if draftPath exists.
// Returns true if draft was loaded.
func loadDraftYAML(draftPath string, out any) (bool, error) {
	draftPath = strings.TrimSpace(draftPath)
	if draftPath == "" {
		return false, nil
	}
	data, err := os.ReadFile(draftPath)
	if err != nil {
		return false, err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return false, fmt.Errorf("parse draft: %w", err)
	}
	return true, nil
}

// startYAMLDraftHandler standardizes Ctrl+C draft-save behavior for wizard flows.
// It stores plaintext YAML draft to tmp/ and restores global signal handler.
func startYAMLDraftHandler(targetPath, draftPath string, state any, isEmpty func() bool) func() {
	return startDraftInterruptHandler(targetPath, draftPath, func() ([]byte, bool) {
		if isEmpty != nil && isEmpty() {
			return nil, false
		}
		data, err := yaml.Marshal(state)
		if err != nil {
			return nil, false
		}
		return data, true
	})
}

// latestDraftForTarget returns the newest draft path for a target config file.
func latestDraftForTarget(targetPath string) string {
	base := filepath.Base(targetPath)
	pattern := filepath.Join("tmp", fmt.Sprintf("%s.draft.*.yaml", base))
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return ""
	}
	type info struct {
		path string
		mod  int64
	}
	var files []info
	for _, p := range matches {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		files = append(files, info{path: p, mod: st.ModTime().UnixNano()})
	}
	if len(files) == 0 {
		return ""
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod > files[j].mod })
	return files[0].path
}
