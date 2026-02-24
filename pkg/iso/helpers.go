package iso

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

// newGovcCmd creates a govc command with vCenter authentication environment variables.
// Centralizes all govc invocations - avoids duplicating GOVC_URL + GOVC_INSECURE setup.
func newGovcCmd(ctx context.Context, vcenterHost, vcenterUser, vcenterPass string, insecure bool, args ...string) *exec.Cmd {
	insecureVal := "false"
	if insecure {
		insecureVal = "true"
	}
	cmd := exec.CommandContext(ctx, "govc", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOVC_URL=https://%s/sdk", vcenterHost),
		fmt.Sprintf("GOVC_USERNAME=%s", vcenterUser),
		fmt.Sprintf("GOVC_PASSWORD=%s", vcenterPass),
		fmt.Sprintf("GOVC_INSECURE=%s", insecureVal),
	)
	return cmd
}

// getDevices returns all virtual devices of a VM.
// Single error-handling location for vm.Device() calls.
func getDevices(ctx context.Context, vm *object.VirtualMachine) (object.VirtualDeviceList, error) {
	devices, err := vm.Device(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM devices: %w", err)
	}
	return devices, nil
}

// getCDROMs returns all CD-ROM devices from a device list.
func getCDROMs(devices object.VirtualDeviceList) []*types.VirtualCdrom {
	raw := devices.SelectByType((*types.VirtualCdrom)(nil))
	result := make([]*types.VirtualCdrom, 0, len(raw))
	for _, d := range raw {
		if cdrom, ok := d.(*types.VirtualCdrom); ok {
			result = append(result, cdrom)
		}
	}
	return result
}

// reconfigureVM applies device changes to a VM and waits for completion.
// Single place for the reconfigure+wait pattern used throughout manager.go.
func reconfigureVM(ctx context.Context, vm *object.VirtualMachine, changes []types.BaseVirtualDeviceConfigSpec) error {
	spec := types.VirtualMachineConfigSpec{DeviceChange: changes}
	task, err := vm.Reconfigure(ctx, spec)
	if err != nil {
		return fmt.Errorf("failed to reconfigure VM: %w", err)
	}
	if err := task.Wait(ctx); err != nil {
		return fmt.Errorf("reconfigure task failed: %w", err)
	}
	return nil
}
