package cgutil

import (
	"context"
	"fmt"

	"github.com/hashicorp/nomad/lib/cpuset"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/opencontainers/runc/libcontainer/cgroups"
)

// CpusetManager is used to setup cpuset cgroups for each task. A pool of shared cpus is managed for
// tasks which don't require any reserved cores and a cgroup is managed secretly for each task which
// require reserved cores.
type CpusetManager interface {
	// Init should be called before any tasks are managed to ensure the cgroup parent exists and
	// check that proper permissions are granted to manage cgroups.
	Init() error

	// AddAlloc adds an allocation to the manager
	AddAlloc(alloc *structs.Allocation)

	// RemoveAlloc removes an alloc by ID from the manager
	RemoveAlloc(allocID string)

	// CgroupPathFor returns a callback for getting the cgroup path and any error that may have occurred during
	// cgroup initialization. The callback will block if the cgroup has not been created
	CgroupPathFor(allocID, taskName string) CgroupPathGetter
}

type NoopCpusetManager struct{}

func (n NoopCpusetManager) Init() error {
	return nil
}

func (n NoopCpusetManager) AddAlloc(alloc *structs.Allocation) {
}

func (n NoopCpusetManager) RemoveAlloc(allocID string) {
}

func (n NoopCpusetManager) CgroupPathFor(allocID, task string) CgroupPathGetter {
	return func(context.Context) (string, error) { return "", nil }
}

// CgroupPathGetter is a function which returns the cgroup path and any error which ocured during cgroup initialization.
// It should block until the cgroup has been created or an error is reported
type CgroupPathGetter func(context.Context) (path string, err error)

type TaskCgroupInfo struct {
	CgroupPath         string
	RelativeCgroupPath string
	Cpuset             cpuset.CPUSet
	Error              error
}

func getCPUsFromCgroupV2(group string) ([]uint16, error) {
	path := rootedV2(group)

	fmt.Println("SH getCPUsFromCgroupV2, group:", group, "path:", path)

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

func getParentV2(parent string) string {
	if parent == "" {
		return DefaultCgroupV2Parent
	}
	return parent
}
