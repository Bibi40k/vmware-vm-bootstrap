// vmbootstrap - CLI tool for managing and bootstrapping VMs in VMware vCenter
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

var vcenterConfigFile string
var debugLogs bool
var bootstrapResultPath string
var nodeVMConfigPath string
var nodeToVersion string
var nodeTalosconfig string
var nodeEndpoint string
var nodePreserve bool
var nodeInsecure bool

// mainSigCh receives SIGINT for the default (non-bootstrap) handler.
// bootstrapVM temporarily stops delivery to this channel so it can handle
// Ctrl+C itself (cancel context + offer VM cleanup).
var mainSigCh = make(chan os.Signal, 1)

var rootCmd = &cobra.Command{
	Use:           "vmbootstrap",
	Short:         "Manage and bootstrap OS-profile VMs in VMware vCenter",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		_ = initDebugLogger()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		return runManager()
	},
}

var runCmd = &cobra.Command{
	Use:           "run",
	Short:         "Select a VM config and bootstrap it",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		return runBootstrapSelector()
	},
}

var smokeCmd = &cobra.Command{
	Use:           "smoke",
	Short:         "Bootstrap and run a minimal post-install smoke test",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		vmPath, _ := cmd.Flags().GetString("config")
		if vmPath == "" {
			selectedPath, selectedLabel, err := selectVMConfig("\033[1mvmbootstrap\033[0m — Smoke Test", "Select VM config to smoke test:")
			if err != nil {
				return err
			}
			if selectedPath == "" {
				return nil
			}
			if !readYesNo(fmt.Sprintf("Run smoke test for %s?", selectedLabel), true) {
				fmt.Println("  Cancelled.")
				return nil
			}
			vmPath = selectedPath
		}
		cleanup, _ := cmd.Flags().GetBool("cleanup")
		return smokeVM(vmPath, cleanup)
	},
}

var nodeCmd = &cobra.Command{
	Use:           "node",
	Short:         "Node lifecycle operations (create/delete/recreate/update)",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var nodeCreateCmd = &cobra.Command{
	Use:           "create",
	Short:         "Create node from VM config",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		return nodeCreate(nodeVMConfigPath)
	},
}

var nodeDeleteCmd = &cobra.Command{
	Use:           "delete",
	Short:         "Delete node from VM config",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		return nodeDelete(nodeVMConfigPath)
	},
}

var nodeRecreateCmd = &cobra.Command{
	Use:           "recreate",
	Short:         "Delete and create node from VM config",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		return nodeRecreate(nodeVMConfigPath)
	},
}

var nodeUpdateCmd = &cobra.Command{
	Use:           "update",
	Short:         "Upgrade Talos node OS via talosctl",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		return nodeUpdateTalos(nodeVMConfigPath, nodeToVersion, nodeTalosconfig, nodeEndpoint, nodePreserve, nodeInsecure)
	},
}

var talosCmd = &cobra.Command{
	Use:           "talos",
	Short:         "Talos profile utilities (schematic manager)",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var talosConfigCmd = &cobra.Command{
	Use:           "config",
	Short:         "Create/update Talos Image Factory schematics",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRequirements(); err != nil {
			return err
		}
		return runTalosConfigWizard()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&vcenterConfigFile, "vcenter-config", "configs/vcenter.sops.yaml",
		"Path to vCenter config file (SOPS encrypted)")
	rootCmd.PersistentFlags().BoolVar(&debugLogs, "debug", false, "Enable debug logging to tmp/vmbootstrap-debug.log")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(smokeCmd)
	rootCmd.AddCommand(nodeCmd)
	rootCmd.AddCommand(talosCmd)
	nodeCmd.AddCommand(nodeCreateCmd)
	nodeCmd.AddCommand(nodeDeleteCmd)
	nodeCmd.AddCommand(nodeRecreateCmd)
	nodeCmd.AddCommand(nodeUpdateCmd)
	talosCmd.AddCommand(talosConfigCmd)

	runCmd.Flags().StringVar(&bootstrapResultPath, "bootstrap-result", "",
		"Write bootstrap result to YAML/JSON file (optional)")

	smokeCmd.Flags().String("config", "", "Path to VM config file (SOPS encrypted)")
	smokeCmd.Flags().Bool("cleanup", false, "Delete VM after smoke test")

	nodeCmd.PersistentFlags().StringVar(&nodeVMConfigPath, "config", "", "Path to VM config file (SOPS encrypted)")
	nodeUpdateCmd.Flags().StringVar(&nodeToVersion, "to-version", "", "Talos target version (defaults to vm.profiles.talos.version)")
	nodeUpdateCmd.Flags().StringVar(&nodeTalosconfig, "talosconfig", "", "Path to talosconfig file (optional)")
	nodeUpdateCmd.Flags().StringVar(&nodeEndpoint, "endpoint", "", "Talos API endpoint (defaults to vm.ip_address)")
	nodeUpdateCmd.Flags().BoolVar(&nodePreserve, "preserve", false, "Preserve /var in Talos upgrade")
	nodeUpdateCmd.Flags().BoolVar(&nodeInsecure, "insecure", false, "Pass --insecure to talosctl")
}

func main() {
	// Handle Ctrl+C — print a clean message and exit 0 so make doesn't show "*** Interrupt".
	// bootstrapVM temporarily calls signal.Stop(mainSigCh) to intercept Ctrl+C itself.
	signal.Notify(mainSigCh, os.Interrupt)
	go func() {
		<-mainSigCh
		restoreTTYOnExit()
		fmt.Println("\nCancelled.")
		os.Exit(0)
	}()

	if err := rootCmd.Execute(); err != nil {
		const (
			red    = "\033[31m"
			yellow = "\033[33m"
			cyan   = "\033[36m"
			reset  = "\033[0m"
		)
		if ue, ok := err.(*userError); ok {
			fmt.Fprintf(os.Stderr, "%sError:%s %s\n", red, reset, ue.Error())
			if hint := ue.Hint(); hint != "" {
				fmt.Fprintf(os.Stderr, "%sHint:%s %s%s%s\n", yellow, reset, cyan, hint, reset)
			}
		} else {
			fmt.Fprintf(os.Stderr, "%sError:%s %v\n", red, reset, err)
		}
		if debugCleanup != nil {
			debugCleanup()
		}
		os.Exit(1)
	}
	if debugCleanup != nil {
		debugCleanup()
	}
}
