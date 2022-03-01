package cgutil

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs2"
	"github.com/opencontainers/runc/libcontainer/configs"
)

const (
	DefaultCgroupV2Parent = "nomad.slice"

	// cgroupV2root is hard-coded in the cgroups.v2 specification
	cgroupV2root = "/sys/fs/cgroup"

	// isRootless is (for now) always false; Nomad clients require root
	isRootless = false

	// v2CreationPID is a special PID in libcontainer/cgroups used to denote a
	// cgroup
	v2CreationPID = -1
)

func rootedV2(group string) string {
	return filepath.Join(cgroupV2root, group)
}

func NewCpusetManagerV2(parent string, logger hclog.Logger) CpusetManager {
	return &cpusetManagerV2{
		ctx:    context.TODO(),
		parent: getParentV2(parent),
		logger: logger,
	}
}

type cpusetManagerV2 struct {
	ctx    context.Context
	logger hclog.Logger
	parent string
	mgr    cgroups.Manager
}

func (c *cpusetManagerV2) Init() error {
	fmt.Println("SH mgr V2 Iinit")
	if err := c.initHierarchy(); err != nil {
		c.logger.Error("failed to init cpuset manager", "err", err)
		return err
	}

	return nil
}

func (c *cpusetManagerV2) AddAlloc(alloc *structs.Allocation) {
	panic("implement me")
}

func (c *cpusetManagerV2) RemoveAlloc(allocID string) {
	panic("implement me")
}

func (c *cpusetManagerV2) CgroupPathFor(allocID, taskName string) CgroupPathGetter {
	panic("implement me")
}

func (c *cpusetManagerV2) getParent() string {
	return rootedV2(c.parent)
}

func (c *cpusetManagerV2) initHierarchy() error {
	cfg := &configs.Cgroup{}

	parent := c.getParent()

	mgr, err := fs2.NewManager(cfg, parent, isRootless)
	if err != nil {
		return fmt.Errorf("failed to setup cgroup hierarchy: %w", err)
	}

	c.mgr = mgr

	if err = c.mgr.Apply(v2CreationPID); err != nil {
		return fmt.Errorf("failed to apply initial cgroup hierarchy: %w", err)
	}

	c.logger.Debug("established cgroup hierarchy", "parent", parent)
	return nil
}
