package talos

import (
	"context"
	"fmt"

	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/profile"
)

type Provisioner struct{}

func New() *Provisioner { return &Provisioner{} }

func (p *Provisioner) Name() string { return "talos" }

func (p *Provisioner) ProvisionAndBoot(ctx context.Context, in profile.Input, rt profile.Runtime) (profile.Result, error) {
	_ = ctx
	_ = in
	_ = rt
	return profile.Result{}, fmt.Errorf("talos profile is not implemented yet in vmbootstrap bootstrap flow; use node lifecycle flow")
}

func (p *Provisioner) PostInstall(ctx context.Context, in profile.Input, rt profile.Runtime, res profile.Result) error {
	_ = ctx
	_ = in
	_ = rt
	_ = res
	return nil
}
