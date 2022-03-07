package cgutil

import (
	"context"
	"fmt"

	"github.com/hashicorp/nomad/lib/cpuset"
	"github.com/hashicorp/nomad/nomad/structs"
)

// CpusetManager is used to setup cpuset cgroups for each task.
type CpusetManager interface {
	// Init should be called with the initial set of reservable cores before any
	// allocations are managed. Ensures the parent cgroup exists and proper permissions
	// are available for managing cgroups.
	Init([]uint16) error

	// AddAlloc adds an allocation to the manager
	AddAlloc(alloc *structs.Allocation)

	// RemoveAlloc removes an alloc by ID from the manager
	RemoveAlloc(allocID string)

	// CgroupPathFor returns a callback for getting the cgroup path and any error that may have occurred during
	// cgroup initialization. The callback will block if the cgroup has not been created
	CgroupPathFor(allocID, taskName string) CgroupPathGetter
}

type NoopCpusetManager struct{}

func (n NoopCpusetManager) Init([]uint16) error {
	return nil
}

func (n NoopCpusetManager) AddAlloc(alloc *structs.Allocation) {
}

func (n NoopCpusetManager) RemoveAlloc(allocID string) {
}

func (n NoopCpusetManager) CgroupPathFor(allocID, task string) CgroupPathGetter {
	return func(context.Context) (string, error) { return "", nil }
}

// CgroupPathGetter is a function which returns the cgroup path and any error which
// occurred during cgroup initialization.
//
// It should block until the cgroup has been created or an error is reported.
type CgroupPathGetter func(context.Context) (path string, err error)

type TaskCgroupInfo struct {
	AllocID            string // v2 only
	Task               string // v2 only
	CgroupPath         string
	RelativeCgroupPath string
	Cpuset             cpuset.CPUSet
	Error              error
}

func (t TaskCgroupInfo) ID() string {
	return makeID(t.AllocID, t.Task)
}

func makeID(allocID, task string) string {
	return fmt.Sprintf("%s.%s", allocID, task)
}

func makeScope(id string) string {
	return id + ".scope"
}
