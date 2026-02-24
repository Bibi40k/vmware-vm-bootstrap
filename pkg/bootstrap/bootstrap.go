package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Bibi40k/vmware-vm-bootstrap/configs"
	"github.com/Bibi40k/vmware-vm-bootstrap/internal/utils"
	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/cloudinit"
	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/iso"
	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/vcenter"
	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/vm"
	"github.com/google/uuid"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// defaultLogger is used if no logger is provided.
var defaultLogger = slog.Default()

// sshVerifier is used by VM.Verify to allow tests to stub SSH checks.
var sshVerifier = verifySSHAccess

// bootstrapper holds injectable service factories.
// Production code uses defaultBootstrapper(); tests inject mocks.
type bootstrapper struct {
	connectVCenter func(ctx context.Context, cfg *VMConfig) (vcenter.ClientInterface, error)
	newVMCreator   func(ctx context.Context) vm.CreatorInterface
	newISOManager  func(ctx context.Context) iso.ManagerInterface
	waitInstall    func(ctx context.Context, vmObj *object.VirtualMachine, cfg *VMConfig, logger *slog.Logger) error
	checkSSH       func(ctx context.Context, ipAddr string) error
}

// defaultBootstrapper returns a bootstrapper with real production implementations.
func defaultBootstrapper() *bootstrapper {
	return &bootstrapper{
		connectVCenter: func(ctx context.Context, cfg *VMConfig) (vcenter.ClientInterface, error) {
			return vcenter.NewClient(ctx, &vcenter.Config{
				Host:     cfg.VCenterHost,
				Username: cfg.VCenterUsername,
				Password: cfg.VCenterPassword,
				Port:     cfg.VCenterPort,
				Insecure: cfg.VCenterInsecure,
			})
		},
		newVMCreator: func(ctx context.Context) vm.CreatorInterface {
			return vm.NewCreator(ctx)
		},
		newISOManager: func(ctx context.Context) iso.ManagerInterface {
			return iso.NewManager(ctx)
		},
		waitInstall: waitForInstallation,
		checkSSH:    verifySSHAccess,
	}
}

// Bootstrap creates and configures a complete VM in vCenter.
// Returns VM object ONLY after:
// - VM created in vCenter
// - Ubuntu installed with cloud-init
// - User SSH accessible (verified through TCP port 22)
func Bootstrap(ctx context.Context, cfg *VMConfig) (*VM, error) {
	return BootstrapWithLogger(ctx, cfg, defaultLogger)
}

// BootstrapWithLogger creates and configures a VM with custom logger.
func BootstrapWithLogger(ctx context.Context, cfg *VMConfig, logger *slog.Logger) (*VM, error) {
	return defaultBootstrapper().run(ctx, cfg, logger)
}

// run is the internal implementation, testable via injected dependencies.
func (b *bootstrapper) run(ctx context.Context, cfg *VMConfig, logger *slog.Logger) (*VM, error) {
	// STEP 1: Validate and set defaults
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	logger.Info("Starting VM bootstrap",
		"name", cfg.Name,
		"vcenter", cfg.VCenterHost,
		"datacenter", cfg.Datacenter,
	)

	// STEP 2: Connect to vCenter
	vclient, err := b.connectVCenter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("vCenter connection failed: %w", err)
	}
	defer func() {
		_ = vclient.Disconnect()
	}()

	logger.Info("Connected to vCenter", "host", cfg.VCenterHost)

	// STEP 3: Check if VM already exists (idempotency)
	existingVM, err := vclient.FindVM(cfg.Datacenter, cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing VM: %w", err)
	}
	if existingVM != nil {
		return nil, fmt.Errorf("VM %q already exists", cfg.Name)
	}

	// STEP 4: Find vCenter objects
	folder, err := vclient.FindFolder(cfg.Datacenter, cfg.Folder)
	if err != nil {
		return nil, fmt.Errorf("failed to find folder: %w", err)
	}

	resourcePool, err := vclient.FindResourcePool(cfg.Datacenter, cfg.ResourcePool)
	if err != nil {
		return nil, fmt.Errorf("failed to find resource pool: %w", err)
	}

	datastore, err := vclient.FindDatastore(cfg.Datacenter, cfg.Datastore)
	if err != nil {
		return nil, fmt.Errorf("failed to find datastore: %w", err)
	}

	// isoDatastore is where Ubuntu + NoCloud ISOs are uploaded.
	// Defaults to the VM datastore when ISODatastore is not configured.
	isoDatastoreName := cfg.ISODatastore
	if isoDatastoreName == "" {
		isoDatastoreName = cfg.Datastore
	}
	isoDatastore, err := vclient.FindDatastore(cfg.Datacenter, isoDatastoreName)
	if err != nil {
		return nil, fmt.Errorf("failed to find ISO datastore %q: %w", isoDatastoreName, err)
	}

	network, err := vclient.FindNetwork(cfg.Datacenter, cfg.NetworkName)
	if err != nil {
		return nil, fmt.Errorf("failed to find network: %w", err)
	}

	logger.Info("vCenter objects located",
		"folder", cfg.Folder,
		"datastore", cfg.Datastore,
		"iso_datastore", isoDatastoreName,
		"network", cfg.NetworkName,
	)

	// STEP 5: Create VM hardware
	creator := b.newVMCreator(ctx)

	vmConfig := &vm.Config{
		Name:         cfg.Name,
		CPUs:         int32(cfg.CPUs),
		MemoryMB:     int64(cfg.MemoryMB),
		DiskSizeGB:   int64(cfg.DiskSizeGB),
		NetworkName:  cfg.NetworkName,
		Datacenter:   cfg.Datacenter,
		Folder:       cfg.Folder,
		ResourcePool: cfg.ResourcePool,
		Datastore:    cfg.Datastore,
		Firmware:     cfg.Firmware,
	}

	if cfg.DataDiskSizeGB != nil {
		size := int64(*cfg.DataDiskSizeGB)
		vmConfig.DataDiskSizeGB = &size
	}

	spec := creator.CreateSpec(vmConfig)
	createdVM, err := creator.Create(folder, resourcePool, datastore, spec)
	if err != nil {
		return nil, fmt.Errorf("VM creation failed: %w", err)
	}

	// Initialize ISO manager early (needed in defer cleanup)
	isoMgr := b.newISOManager(ctx)

	// Cleanup partial VM and uploaded ISOs on failure (idempotency)
	var bootstrapSuccess bool
	var nocloudUploadPath string // set after upload, used in cleanup
	defer func() {
		if !bootstrapSuccess && !cfg.SkipCleanupOnError {
			if createdVM != nil {
				logger.Warn("Bootstrap failed - cleaning up partial VM", "name", cfg.Name)
				if deleteErr := creator.Delete(createdVM); deleteErr != nil {
					logger.Error("Failed to cleanup partial VM", "error", deleteErr)
				}
			}
			if nocloudUploadPath != "" {
				if deleteErr := isoMgr.DeleteFromDatastore(isoDatastoreName, nocloudUploadPath,
					cfg.VCenterHost, cfg.VCenterUsername, cfg.VCenterPassword, cfg.VCenterInsecure); deleteErr != nil {
					logger.Warn("Failed to cleanup NoCloud ISO from datastore", "error", deleteErr)
				} else {
					logger.Info("NoCloud ISO cleaned up from datastore", "path", nocloudUploadPath)
				}
			}
		}
	}()

	logger.Info("VM hardware created", "name", cfg.Name)

	// STEP 6: Add SCSI controller
	scsiKey, err := creator.EnsureSCSIController(createdVM)
	if err != nil {
		return nil, fmt.Errorf("failed to add SCSI controller: %w", err)
	}

	// STEP 7: Add OS disk
	if err := creator.AddDisk(createdVM, datastore, int64(cfg.DiskSizeGB), scsiKey); err != nil {
		return nil, fmt.Errorf("failed to add OS disk: %w", err)
	}

	// STEP 8: Add data disk (if specified)
	if cfg.DataDiskSizeGB != nil {
		if err := creator.AddDisk(createdVM, datastore, int64(*cfg.DataDiskSizeGB), scsiKey); err != nil {
			return nil, fmt.Errorf("failed to add data disk: %w", err)
		}
		logger.Info("Data disk added", "size_gb", *cfg.DataDiskSizeGB)
	}

	// STEP 9: Add network adapter
	if err := creator.AddNetworkAdapter(createdVM, network); err != nil {
		return nil, fmt.Errorf("failed to add network adapter: %w", err)
	}

	logger.Info("VM hardware configuration complete")

	// STEP 10: Generate cloud-init configs
	generator, err := cloudinit.NewGenerator()
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud-init generator: %w", err)
	}

	// Resolve password hash: PasswordHash takes priority, then auto-hash Password
	var passwordHash string
	switch {
	case cfg.PasswordHash != "":
		passwordHash = cfg.PasswordHash
	case cfg.Password != "":
		hashed, hashErr := utils.HashPasswordBcrypt(cfg.Password)
		if hashErr != nil {
			return nil, fmt.Errorf("failed to hash password: %w", hashErr)
		}
		passwordHash = hashed
	default:
		// No password - SSH key-only access
		passwordHash = "*"
	}

	// Calculate CIDR before generating configs (needed in user-data and network-config)
	cidr, err := utils.NetmaskToCIDR(cfg.Netmask)
	if err != nil {
		return nil, fmt.Errorf("invalid netmask: %w", err)
	}

	// Compute swap size: per-VM override takes priority, else use default from configs/defaults.yaml
	swapSizeGB := configs.Defaults.CloudInit.SwapSizeGB
	if cfg.SwapSizeGB != nil {
		swapSizeGB = *cfg.SwapSizeGB
	}
	swapSize := fmt.Sprintf("%dG", swapSizeGB)

	// Generate user-data
	userData, err := generator.GenerateUserData(&cloudinit.UserDataInput{
		Hostname:          cfg.Name,
		Username:          cfg.Username,
		PasswordHash:      passwordHash,
		SSHPublicKeys:     cfg.SSHPublicKeys,
		AllowPasswordSSH:  cfg.AllowPasswordSSH,
		Locale:            cfg.Locale,
		Timezone:          cfg.Timezone,
		KeyboardLayout:    configs.Defaults.CloudInit.KeyboardLayout,
		SwapSize:          swapSize,
		Packages:          configs.Defaults.CloudInit.Packages,
		UserGroups:        configs.Defaults.CloudInit.UserGroups,
		UserShell:         configs.Defaults.CloudInit.UserShell,
		DataDiskMountPath: cfg.DataDiskMountPath,
		IPAddress:         cfg.IPAddress,
		CIDR:              cidr,
		Gateway:           cfg.Gateway,
		DNS:               cfg.DNS,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate user-data: %w", err)
	}

	// Generate meta-data
	metaData, err := generator.GenerateMetaData(&cloudinit.MetaDataInput{
		InstanceID: uuid.New().String(),
		Hostname:   cfg.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate meta-data: %w", err)
	}

	// Generate network-config
	networkConfig, err := generator.GenerateNetworkConfig(&cloudinit.NetworkConfigInput{
		InterfaceName: configs.Defaults.Network.Interface,
		IPAddress:     cfg.IPAddress,
		CIDR:          cidr,
		Gateway:       cfg.Gateway,
		DNS:           cfg.DNS,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate network-config: %w", err)
	}

	logger.Info("Cloud-init configs generated")

	// STEP 11: Download Ubuntu ISO
	ubuntuISOPath, err := isoMgr.DownloadUbuntu(cfg.UbuntuVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to download Ubuntu ISO: %w", err)
	}

	logger.Info("Ubuntu ISO ready", "path", ubuntuISOPath)

	// STEP 11.5: Modify Ubuntu ISO for autoinstall (kernel param + timeout)
	// wasCreated=true means ISO was rebuilt → must force overwrite on datastore
	ubuntuISOPath, _, err = isoMgr.ModifyUbuntuISO(ubuntuISOPath)
	if err != nil {
		return nil, fmt.Errorf("failed to modify Ubuntu ISO: %w", err)
	}

	logger.Info("Ubuntu ISO modified for autoinstall", "path", ubuntuISOPath)

	// STEP 12: Create NoCloud ISO
	nocloudISOPath, err := isoMgr.CreateNoCloudISO(userData, metaData, networkConfig, cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create NoCloud ISO: %w", err)
	}

	logger.Info("NoCloud ISO created", "path", nocloudISOPath)

	// STEP 13: Upload ISOs to datastore
	ubuntuUploadPath := fmt.Sprintf("ISO/ubuntu/%s", filepath.Base(ubuntuISOPath))
	nocloudUploadPath = fmt.Sprintf("ISO/nocloud/%s", filepath.Base(nocloudISOPath))

	if err := isoMgr.UploadToDatastore(isoDatastore, ubuntuISOPath, ubuntuUploadPath,
		cfg.VCenterHost, cfg.VCenterUsername, cfg.VCenterPassword, cfg.VCenterInsecure); err != nil {
		return nil, fmt.Errorf("failed to upload Ubuntu ISO: %w", err)
	}

	// NoCloud ISO: always upload (VM-specific, small 1MB, matches Python behavior)
	if err := isoMgr.UploadAlways(isoDatastore, nocloudISOPath, nocloudUploadPath,
		cfg.VCenterHost, cfg.VCenterUsername, cfg.VCenterPassword, cfg.VCenterInsecure); err != nil {
		return nil, fmt.Errorf("failed to upload NoCloud ISO: %w", err)
	}

	// Cleanup NoCloud ISO after upload (VM-specific, no longer needed locally)
	_ = os.Remove(nocloudISOPath)
	logger.Info("NoCloud ISO uploaded and cleaned up")

	logger.Info("ISOs uploaded to datastore")

	// STEP 14: Mount ISOs to VM
	ubuntuMountPath := fmt.Sprintf("[%s] %s", isoDatastoreName, ubuntuUploadPath)
	nocloudMountPath := fmt.Sprintf("[%s] %s", isoDatastoreName, nocloudUploadPath)

	if err := isoMgr.MountISOs(createdVM, ubuntuMountPath, nocloudMountPath); err != nil {
		return nil, fmt.Errorf("failed to mount ISOs: %w", err)
	}

	logger.Info("ISOs mounted to VM")

	// STEP 15: Power on VM
	if err := creator.PowerOn(createdVM); err != nil {
		return nil, fmt.Errorf("failed to power on VM: %w", err)
	}

	logger.Info("VM powered on - waiting for installation...")

	// STEP 15.5: Verify CD-ROMs are connected after boot (matches Python Step 9)
	if err := isoMgr.EnsureCDROMsConnectedAfterBoot(createdVM); err != nil {
		logger.Warn("CD-ROM post-boot check failed (continuing)", "error", err)
	}

	// STEP 16: Wait for installation to complete
	if err := b.waitInstall(ctx, createdVM, cfg, logger); err != nil {
		return nil, fmt.Errorf("installation failed: %w", err)
	}

	logger.Info("Installation complete")

	// STEP 16.5: Cleanup ISOs from datastore
	logger.Info("Powering off VM to release CD-ROM file locks...")
	if err := creator.PowerOff(createdVM); err != nil {
		logger.Warn("Failed to power off VM for cleanup (continuing)", "error", err)
	} else {
		if err := isoMgr.RemoveAllCDROMs(createdVM); err != nil {
			logger.Warn("Failed to remove CD-ROMs (continuing)", "error", err)
		}

		if err := isoMgr.DeleteFromDatastore(isoDatastoreName, nocloudUploadPath,
			cfg.VCenterHost, cfg.VCenterUsername, cfg.VCenterPassword, cfg.VCenterInsecure); err != nil {
			logger.Warn("Failed to delete NoCloud ISO from datastore (non-critical)", "error", err)
		} else {
			logger.Info("NoCloud ISO deleted from datastore", "path", nocloudUploadPath)
		}

		logger.Info("Powering VM back on...")
		if err := creator.PowerOn(createdVM); err != nil {
			return nil, fmt.Errorf("failed to power VM back on after cleanup: %w", err)
		}
	}

	// STEP 17: Verify SSH access
	if cfg.SkipSSHVerify {
		logger.Warn("Skipping SSH verification (SkipSSHVerify=true)")
	} else {
		if err := b.checkSSH(ctx, cfg.IPAddress); err != nil {
			return nil, fmt.Errorf("SSH verification failed: %w", err)
		}
		logger.Info("SSH access verified")
	}

	// Mark bootstrap as successful (prevents defer cleanup)
	bootstrapSuccess = true

	return &VM{
		Name:            cfg.Name,
		IPAddress:       cfg.IPAddress,
		ManagedObject:   createdVM.Reference(),
		SSHReady:        !cfg.SkipSSHVerify,
		Hostname:        cfg.Name,
		VCenterHost:     cfg.VCenterHost,
		VCenterPort:     cfg.VCenterPort,
		VCenterUser:     cfg.VCenterUsername,
		VCenterPass:     cfg.VCenterPassword,
		VCenterInsecure: cfg.VCenterInsecure,
	}, nil
}

// Verify performs a basic health check: VM powered on, VMware Tools running (if available),
// hostname matches (if available), and SSH port is reachable (if IP is set).
func (vm *VM) Verify(ctx context.Context) error {
	client, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     vm.VCenterHost,
		Port:     vm.VCenterPort,
		Username: vm.VCenterUser,
		Password: vm.VCenterPass,
		Insecure: vm.VCenterInsecure,
	})
	if err != nil {
		return fmt.Errorf("vCenter connection failed: %w", err)
	}
	defer func() {
		_ = client.Disconnect()
	}()

	vmObj := object.NewVirtualMachine(client.Client().Client, vm.ManagedObject)
	var moVM mo.VirtualMachine
	if err := vmObj.Properties(ctx, vmObj.Reference(), []string{"runtime", "guest"}, &moVM); err != nil {
		return fmt.Errorf("failed to fetch VM properties: %w", err)
	}

	if moVM.Runtime.PowerState != "poweredOn" {
		return fmt.Errorf("VM not powered on (state=%s)", moVM.Runtime.PowerState)
	}

	if moVM.Guest == nil {
		return fmt.Errorf("guest info unavailable (VMware Tools not reporting)")
	}

	if moVM.Guest.ToolsRunningStatus != "guestToolsRunning" {
		return fmt.Errorf("VMware Tools not running (status=%s)", moVM.Guest.ToolsRunningStatus)
	}

	if vm.Hostname != "" && moVM.Guest.HostName != "" && moVM.Guest.HostName != vm.Hostname {
		return fmt.Errorf("hostname mismatch (expected=%s got=%s)", vm.Hostname, moVM.Guest.HostName)
	}

	if vm.IPAddress == "" {
		return fmt.Errorf("IPAddress is required for SSH verification")
	}

	if err := sshVerifier(ctx, vm.IPAddress); err != nil {
		return fmt.Errorf("SSH verification failed: %w", err)
	}

	return nil
}

// PowerOff powers off the VM and waits for completion.
func (vm *VM) PowerOff(ctx context.Context) error {
	client, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     vm.VCenterHost,
		Port:     vm.VCenterPort,
		Username: vm.VCenterUser,
		Password: vm.VCenterPass,
		Insecure: vm.VCenterInsecure,
	})
	if err != nil {
		return fmt.Errorf("vCenter connection failed: %w", err)
	}
	defer func() {
		_ = client.Disconnect()
	}()

	vmObj := object.NewVirtualMachine(client.Client().Client, vm.ManagedObject)
	state, err := vmObj.PowerState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get power state: %w", err)
	}
	if state != types.VirtualMachinePowerStatePoweredOn {
		return nil
	}

	task, err := vmObj.PowerOff(ctx)
	if err != nil {
		return fmt.Errorf("power off failed: %w", err)
	}
	if err := task.Wait(ctx); err != nil {
		return fmt.Errorf("power off wait failed: %w", err)
	}
	return nil
}

// PowerOn powers on the VM and waits for completion.
func (vm *VM) PowerOn(ctx context.Context) error {
	client, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     vm.VCenterHost,
		Port:     vm.VCenterPort,
		Username: vm.VCenterUser,
		Password: vm.VCenterPass,
		Insecure: vm.VCenterInsecure,
	})
	if err != nil {
		return fmt.Errorf("vCenter connection failed: %w", err)
	}
	defer func() {
		_ = client.Disconnect()
	}()

	vmObj := object.NewVirtualMachine(client.Client().Client, vm.ManagedObject)
	state, err := vmObj.PowerState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get power state: %w", err)
	}
	if state == types.VirtualMachinePowerStatePoweredOn {
		return nil
	}

	task, err := vmObj.PowerOn(ctx)
	if err != nil {
		return fmt.Errorf("power on failed: %w", err)
	}
	if err := task.Wait(ctx); err != nil {
		return fmt.Errorf("power on wait failed: %w", err)
	}
	return nil
}

// Delete powers off the VM if needed and removes it from vCenter.
func (vm *VM) Delete(ctx context.Context) error {
	client, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     vm.VCenterHost,
		Port:     vm.VCenterPort,
		Username: vm.VCenterUser,
		Password: vm.VCenterPass,
		Insecure: vm.VCenterInsecure,
	})
	if err != nil {
		return fmt.Errorf("vCenter connection failed: %w", err)
	}
	defer func() {
		_ = client.Disconnect()
	}()

	vmObj := object.NewVirtualMachine(client.Client().Client, vm.ManagedObject)
	state, err := vmObj.PowerState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get power state: %w", err)
	}
	if state == types.VirtualMachinePowerStatePoweredOn {
		task, err := vmObj.PowerOff(ctx)
		if err != nil {
			return fmt.Errorf("power off failed: %w", err)
		}
		if err := task.Wait(ctx); err != nil {
			return fmt.Errorf("power off wait failed: %w", err)
		}
	}
	task, err := vmObj.Destroy(ctx)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}
	if err := task.Wait(ctx); err != nil {
		return fmt.Errorf("delete wait failed: %w", err)
	}
	return nil
}

// waitForInstallation monitors VM until OS installed.
// Matches Python _wait_for_installation_complete() exactly:
// Phase 1: Wait for VMware Tools running (installation started)
// Phase 2: Wait for VM to reboot (Tools stop = autoinstall complete)
// Phase 3: Wait for Tools running + hostname set (first boot complete)
func waitForInstallation(ctx context.Context, vmObj *object.VirtualMachine, cfg *VMConfig, logger *slog.Logger) error {
	timeout := configs.Defaults.Timeouts.Installation()
	ticker := time.NewTicker(configs.Defaults.Timeouts.Polling())
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)

	toolsWasRunning := false
	rebootDetected := false
	hostnameCheckCount := 0
	requiredHostnameChecks := configs.Defaults.Timeouts.HostnameChecks

	logger.Info("Phase 1: Waiting for installation to start (VMware Tools)...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("installation timeout (%v)", timeout)
			}

			var moVM mo.VirtualMachine
			if err := vmObj.Properties(ctx, vmObj.Reference(), []string{"guest"}, &moVM); err != nil {
				continue
			}
			if moVM.Guest == nil {
				continue
			}

			toolsRunning := moVM.Guest.ToolsRunningStatus == "guestToolsRunning"
			hostname := moVM.Guest.HostName

			if !toolsWasRunning && toolsRunning {
				if !rebootDetected {
					logger.Info("Phase 1 complete: Installation started (VMware Tools running)")
				} else {
					logger.Info("Phase 3: VMware Tools running again after reboot")
				}
				toolsWasRunning = true
			}

			if !toolsWasRunning {
				continue
			}

			if toolsWasRunning && !toolsRunning {
				if !rebootDetected {
					logger.Info("Phase 2: VM rebooting (VMware Tools stopped - autoinstall completing)...")
					rebootDetected = true
				}
				toolsWasRunning = false
				hostnameCheckCount = 0
				continue
			}

			if toolsRunning {
				toolsWasRunning = true
			}

			if rebootDetected && toolsRunning && hostname == cfg.Name {
				hostnameCheckCount++
				logger.Info("Phase 3: Installation may be complete",
					"hostname", hostname,
					"checks", hostnameCheckCount,
					"required", requiredHostnameChecks)
				if hostnameCheckCount >= requiredHostnameChecks {
					logger.Info("Installation complete, waiting for services to start...",
						"wait", configs.Defaults.Timeouts.ServiceStartup())
					time.Sleep(configs.Defaults.Timeouts.ServiceStartup())
					return nil
				}
			} else if rebootDetected && toolsRunning && hostname != cfg.Name {
				hostnameCheckCount = 0
			}

			// Alternative: no reboot but hostname stable (Ubuntu 22.04 behavior)
			if !rebootDetected && toolsRunning && hostname == cfg.Name {
				hostnameCheckCount++
				logger.Info("Installation may be complete (no reboot)",
					"hostname", hostname,
					"checks", hostnameCheckCount)
				if hostnameCheckCount >= requiredHostnameChecks {
					logger.Info("Installation complete (stable hostname), waiting for services...",
						"wait", configs.Defaults.Timeouts.ServiceStartup())
					time.Sleep(configs.Defaults.Timeouts.ServiceStartup())
					return nil
				}
			} else if !rebootDetected && toolsRunning && hostname != cfg.Name {
				hostnameCheckCount = 0
			}
		}
	}
}

// verifySSHAccess verifies SSH port 22 is accessible.
func verifySSHAccess(ctx context.Context, ipAddr string) error {
	t := configs.Defaults.Timeouts
	for i := 0; i < t.SSHRetries; i++ {
		if utils.IsPortOpen(ipAddr, 22, t.SSHConnect()) {
			return nil
		}
		time.Sleep(t.SSHRetryDelay())
	}
	return fmt.Errorf("SSH port 22 not accessible at %s", ipAddr)
}
