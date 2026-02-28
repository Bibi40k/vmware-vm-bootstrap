package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	survey "github.com/AlecAivazis/survey/v2"
)

type menuItem struct {
	label  string
	action func() error
}

func runManager() error {
	for {
		warnings := checkRequiredFiles()

		fmt.Println()
		fmt.Println("\033[1mvmbootstrap\033[0m — Config Manager")
		fmt.Println(strings.Repeat("─", 50))
		if len(warnings) > 0 {
			for _, w := range warnings {
				fmt.Printf("  \033[33m⚠  missing required: %s\033[0m\n", w)
			}
			fmt.Println()
		}

		items := buildMenuItems()

		var labels []string
		for _, it := range items {
			labels = append(labels, it.label)
		}

		var choice string
		if err := survey.AskOne(&survey.Select{
			Message: "Select:",
			Options: labels,
		}, &choice); err != nil {
			return nil // Ctrl+C → clean exit
		}
		// Drain any CPR responses survey left in stdin before any readLine calls.
		drainStdin()

		for _, it := range items {
			if it.label == choice {
				if it.action == nil {
					return nil // Exit
				}
				fmt.Println()
				if err := it.action(); err != nil {
					fmt.Printf("\n\033[31m✗ Error: %v\033[0m\n", err)
					fmt.Print("\nPress Enter to continue...")
					_, _ = stdinReader.ReadString('\n')
				}
				break
			}
		}
	}
}

func buildMenuItems() []menuItem {
	var items []menuItem

	_, vcenterErr := os.Stat("configs/vcenter.sops.yaml")
	vcenterExists := vcenterErr == nil

	if vcenterExists {
		items = append(items, menuItem{
			label:  "[vcenter]  Edit vcenter.sops.yaml",
			action: func() error { return editVCenterConfig("configs/vcenter.sops.yaml") },
		})
	} else {
		items = append(items, menuItem{
			label:  "[+vcenter] Create vcenter.sops.yaml",
			action: func() error { return createVCenterConfig("configs/vcenter.sops.yaml") },
		})
	}

	vmFiles, _ := filepath.Glob("configs/vm.*.sops.yaml")
	for _, path := range vmFiles {
		p := path
		base := filepath.Base(p)
		items = append(items, menuItem{
			label:  "[vm]       Edit " + base,
			action: func() error { return editVMConfig(p) },
		})
	}

	drafts := listDrafts(true)
	for _, d := range drafts {
		draft := d
		items = append(items, menuItem{
			label:  "\033[33m[draft]\033[0m    Resume " + draft.label,
			action: func() error { return resumeDraft(draft) },
		})
		items = append(items, menuItem{
			label:  "\033[31m[draft]\033[0m    Delete " + draft.label,
			action: func() error { return deleteDraft(draft.path) },
		})
	}

	if vcenterExists {
		items = append(items, menuItem{label: "[+vm]      Create new VM", action: runCreateWizard})
	}
	items = append(items, menuItem{label: "           Exit", action: nil})

	return items
}

type draftInfo struct {
	path       string
	targetPath string
	kind       string
	label      string
}

func listDrafts(all bool) []draftInfo {
	matches, _ := filepath.Glob(filepath.Join("tmp", "*.draft.*.yaml"))
	type item struct {
		info    draftInfo
		modTime time.Time
	}
	var items []item
	for _, p := range matches {
		base := filepath.Base(p)
		targetBase := strings.Split(base, ".draft.")[0]
		targetPath := filepath.Join("configs", targetBase)
		kind := "unknown"
		if strings.HasPrefix(targetBase, "vm.") {
			kind = "vm"
		} else if targetBase == "vcenter.sops.yaml" {
			kind = "vcenter"
		}
		fi, _ := os.Stat(p)
		mt := time.Time{}
		if fi != nil {
			mt = fi.ModTime()
		}
		items = append(items, item{
			info: draftInfo{
				path:       p,
				targetPath: targetPath,
				kind:       kind,
				label:      targetBase,
			},
			modTime: mt,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].modTime.After(items[j].modTime) })

	var drafts []draftInfo
	if all {
		for _, it := range items {
			it.info.label = it.info.label + " (" + filepath.Base(it.info.path) + ")"
			drafts = append(drafts, it.info)
		}
		return drafts
	}

	seen := make(map[string]bool)
	for _, it := range items {
		key := it.info.label
		if seen[key] {
			continue
		}
		seen[key] = true
		drafts = append(drafts, it.info)
	}
	return drafts
}

func deleteDraft(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("\n\033[32m✓ Draft deleted: %s\033[0m\n", filepath.Base(path))
	return nil
}

func resumeDraft(d draftInfo) error {
	switch d.kind {
	case "vcenter":
		return createVCenterConfigWithDraft(d.targetPath, d.path)
	case "vm":
		return runCreateWizardWithDraft(d.targetPath, d.path)
	default:
		return fmt.Errorf("unknown draft type: %s", d.label)
	}
}

func runBootstrapSelector() error {
	selectedPath, selectedLabel, err := selectVMConfig("\033[1mvmbootstrap\033[0m — Bootstrap VM", "Select VM config to bootstrap:")
	if err != nil {
		return err
	}
	if selectedPath == "" {
		return nil
	}
	if !readYesNo(fmt.Sprintf("Bootstrap %s?", selectedLabel), true) {
		fmt.Println("  Cancelled.")
		return nil
	}
	return bootstrapVM(selectedPath, bootstrapResultPath)
}

func selectVMConfig(title, prompt string) (string, string, error) {
	vmFiles, _ := filepath.Glob("configs/vm.*.sops.yaml")
	if len(vmFiles) == 0 {
		fmt.Println("\n  No VM configs found in configs/vm.*.sops.yaml")
		fmt.Println("  Run: make config → Create new VM")
		return "", "", nil
	}

	labels := make([]string, len(vmFiles))
	for i, f := range vmFiles {
		labels[i] = filepath.Base(f)
	}
	options := append([]string{}, labels...)
	options = append(options, "Exit")

	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("─", 50))
	var selected string
	if err := survey.AskOne(&survey.Select{
		Message: prompt,
		Options: options,
		Default: labels[0],
	}, &selected); err != nil {
		return "", "", nil
	}
	// Clear delayed terminal control responses left by survey rendering.
	drainStdin()
	if selected == "Exit" {
		fmt.Println()
		return "", "", nil
	}
	fmt.Println()

	var selectedPath string
	for i, label := range labels {
		if label == selected {
			selectedPath = vmFiles[i]
			break
		}
	}
	return selectedPath, selected, nil
}

func checkRequiredFiles() []string {
	var missing []string
	for _, f := range []string{"vcenter.sops.yaml", "defaults.yaml"} {
		if _, err := os.Stat(filepath.Join("configs", f)); os.IsNotExist(err) {
			missing = append(missing, f)
		}
	}
	return missing
}
