package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	if _, err := os.Stat("configs/vcenter.sops.yaml"); err == nil {
		items = append(items, menuItem{
			label:  "[vcenter]  Edit vcenter.sops.yaml",
			action: func() error { return editVCenterConfig("configs/vcenter.sops.yaml") },
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

	items = append(items,
		menuItem{label: "[+]        Create new VM", action: runCreateWizard},
		menuItem{label: "           Exit", action: nil},
	)

	return items
}

func runBootstrapSelector() error {
	vmFiles, _ := filepath.Glob("configs/vm.*.sops.yaml")
	if len(vmFiles) == 0 {
		fmt.Println("\n  No VM configs found in configs/vm.*.sops.yaml")
		fmt.Println("  Run: make config → Create new VM")
		return nil
	}

	// Strip the path prefix for display.
	labels := make([]string, len(vmFiles))
	for i, f := range vmFiles {
		labels[i] = filepath.Base(f)
	}

	fmt.Println()
	fmt.Println("\033[1mvmbootstrap\033[0m — Bootstrap VM")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	selected := interactiveSelect(labels, labels[0], "Select VM config to bootstrap:")
	fmt.Println()

	// Find the full path for the selected label.
	var selectedPath string
	for i, label := range labels {
		if label == selected {
			selectedPath = vmFiles[i]
			break
		}
	}

	if !readYesNo(fmt.Sprintf("Bootstrap %s?", selected), true) {
		fmt.Println("  Cancelled.")
		return nil
	}

	return bootstrapVM(selectedPath)
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
