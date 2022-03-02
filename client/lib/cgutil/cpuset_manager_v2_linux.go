package cgutil

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/lib/cpuset"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs2"
	"github.com/opencontainers/runc/libcontainer/configs"
)

const (
	V2defaultCgroupParent = "nomad.slice"

	// v2CgroupRoot is hard-coded in the cgroups.v2 specification
	v2CgroupRoot = "/sys/fs/cgroup"

	// v2isRootless is (for now) always false; Nomad clients require root
	v2isRootless = false

	// v2CreationPID is a special PID in libcontainer/cgroups used to denote a
	// cgroup
	v2CreationPID = -1
)

func NewCpusetManagerV2(parent string, logger hclog.Logger) CpusetManager {
	return &cpusetManagerV2{
		ctx:    context.TODO(),
		parent: v2GetParent(parent),
		logger: logger,
	}
}

type cpusetManagerV2 struct {
	ctx    context.Context
	logger hclog.Logger
	parent string
	mgr    cgroups.Manager

	lock        sync.Mutex
	allocations map[string]allocTaskCgroupInfo
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
	if alloc == nil || alloc.AllocatedResources == nil {
		return
	}
	c.logger.Trace("add allocation", "name", alloc.Name, "id", alloc.ID)
	//for task, resources := range alloc.AllocatedResources.Tasks {
	//	taskCpuset := cpuset.New(resources.Cpu.ReservedCores...)
	//
	//}
	// alloc.Resources.Cores

}

func (c *cpusetManagerV2) RemoveAlloc(allocID string) {
	panic("implement me")
}

func (c *cpusetManagerV2) CgroupPathFor(allocID, taskName string) CgroupPathGetter {
	panic("implement me")
}

func (c *cpusetManagerV2) getParent() string {
	return v2Root(c.parent)
}

func (c *cpusetManagerV2) initHierarchy() error {
	cfg := &configs.Cgroup{}

	parent := c.getParent()

	mgr, err := fs2.NewManager(cfg, parent, v2isRootless)
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

func v2Root(group string) string {
	return filepath.Join(v2CgroupRoot, group)
}

func v2GetCPUsFromCgroup(group string) ([]uint16, error) {
	path := v2Root(group)

	fmt.Println("SH v2GetCPUsFromCgroup, group:", group, "path:", path)

	effective, err := cgroups.ReadFile(path, "cpuset.cpus.effective")
	if err != nil {
		fmt.Println("A err:", err)
		return nil, err
	}

	fmt.Println("B effective:", effective)
	set, err := cpuset.Parse(effective)
	if err != nil {
		fmt.Println("C: parse:", err)
		return nil, err
	}

	fmt.Println("D: set:", set)

	return set.ToSlice(), nil
}

func v2GetParent(parent string) string {
	if parent == "" {
		return V2defaultCgroupParent
	}
	return parent
}
