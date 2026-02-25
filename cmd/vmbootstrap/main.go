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
var stage1ResultPath string

// mainSigCh receives SIGINT for the default (non-bootstrap) handler.
// bootstrapVM temporarily stops delivery to this channel so it can handle
// Ctrl+C itself (cancel context + offer VM cleanup).
var mainSigCh = make(chan os.Signal, 1)

var rootCmd = &cobra.Command{
	Use:           "vmbootstrap",
	Short:         "Manage and bootstrap Ubuntu VMs in VMware vCenter",
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

func init() {
	rootCmd.PersistentFlags().StringVar(&vcenterConfigFile, "vcenter-config", "configs/vcenter.sops.yaml",
		"Path to vCenter config file (SOPS encrypted)")
	rootCmd.PersistentFlags().BoolVar(&debugLogs, "debug", false, "Enable debug logging to tmp/vmbootstrap-debug.log")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(smokeCmd)

	runCmd.Flags().StringVar(&stage1ResultPath, "stage1-result", "",
		"Write Stage 1 result to YAML/JSON file (optional)")

	smokeCmd.Flags().String("config", "", "Path to VM config file (SOPS encrypted)")
	smokeCmd.Flags().Bool("cleanup", false, "Delete VM after smoke test")
}

func main() {
	// Handle Ctrl+C — print a clean message and exit 0 so make doesn't show "*** Interrupt".
	// bootstrapVM temporarily calls signal.Stop(mainSigCh) to intercept Ctrl+C itself.
	signal.Notify(mainSigCh, os.Interrupt)
	go func() {
		<-mainSigCh
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
