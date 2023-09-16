package parallels

import (
	"runtime"
	"context"
	"errors"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/cirruslabs/cirrus-cli/internal/executor/instance/persistentworker/remoteagent"
	"github.com/cirruslabs/cirrus-cli/internal/executor/instance/runconfig"
	"github.com/cirruslabs/cirrus-cli/internal/executor/platform"
	"github.com/cirruslabs/cirrus-cli/internal/logger"
)

var (
	ErrFailed = errors.New("Parallels isolation failed")
)

type Parallels struct {
	logger      logger.Lightweight
	vmImage     string
	sshUser     string
	sshPassword string
	agentOS     string
}

func New(vmImage, sshUser, sshPassword, agentOS string, opts ...Option) (*Parallels, error) {
	parallels := &Parallels{
		vmImage:     vmImage,
		sshUser:     sshUser,
		sshPassword: sshPassword,
		agentOS:     agentOS,
	}

	// Apply options
	for _, opt := range opts {
		opt(parallels)
	}
	// Apply default options (to cover those that weren't specified)
	if parallels.logger == nil {
		parallels.logger = &logger.LightweightStub{}
	}

	return parallels, nil
}

func (parallels *Parallels) Run(ctx context.Context, config *runconfig.RunConfig) (err error) {
	vm, err := NewVMClonedFrom(ctx, parallels.vmImage)
	if err != nil {
		return fmt.Errorf("%w: failed to create VM cloned from %q: %v", ErrFailed, parallels.vmImage, err)
	}
	defer vm.Close()

	if err := vm.Start(ctx); err != nil {
		return fmt.Errorf("%w: failed to start VM %q: %v", ErrFailed, vm.Ident(), err)
	}

	// Wait for the VM to start and get it's DHCP address
	var ip string

	if err := retry.Do(func() error {
		ip, err = vm.RetrieveIP(ctx)
		return err
	}, retry.Context(ctx), retry.RetryIf(func(err error) bool {
		return errors.Is(err, ErrDHCPSnoopFailed)
	})); err != nil {
		return fmt.Errorf("%w: failed to retrieve VM %q IP-address: %v", ErrFailed, vm.name, err)
	}

	// NOTE:- The agent was previously set only for amd64. this will now use the goarch.
	// Ideally we should read this from the cirrus.yml file and pass it through
	return remoteagent.WaitForAgent(ctx, parallels.logger, ip,
		parallels.sshUser, parallels.sshPassword, parallels.agentOS, runtime.GOARCH,
		config, vm.ClonedFromSuspended(), nil, "")
}

func (parallels *Parallels) WorkingDirectory(projectDir string, dirtyMode bool) string {
	return platform.NewWindows("11").GenericWorkingDir()
}

func (parallels *Parallels) Close() error {
	return nil
}
