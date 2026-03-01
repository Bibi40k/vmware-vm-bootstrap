package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
