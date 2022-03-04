package cgutil

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/helper"
	"github.com/hashicorp/nomad/lib/cpuset"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs2"
)

const (
	// V2defaultCgroupParent is the name of Nomad's default parent cgroup, under which
	// all other cgroups are managed. This can be changed with client configuration
	// in case for e.g. Nomad tasks should be further constrained by an externally
	// configured systemd cgroup.
	V2defaultCgroupParent = "nomad.slice"

	// v2SharedCgroup is the name of Nomad's shared cgroup.
	v2SharedCgroup = "shared.scope"

	// v2ReservedCgroup is the name of Nomad's reserved cgroup.
	//
	// Tasks which reserve cores will have a cgroup created underneath.
	v2ReservedCgroup = "reserved.slice"

	// v2CgroupRoot is hard-coded in the cgroups.v2 specification.
	v2CgroupRoot = "/sys/fs/cgroup"

	// v2isRootless is (for now) always false; Nomad clients require root.
	v2isRootless = false

	// v2CreationPID is a special PID in libcontainer used to denote a cgroup
	// should be created, but with no process added.
	v2CreationPID = -1
)

func NewCpusetManagerV2(parent string, logger hclog.Logger) CpusetManager {
	cgroupParent := v2GetParent(parent)
	return &cpusetManagerV2{
		ctx:       context.TODO(),
		parent:    cgroupParent,
		parentAbs: filepath.Join(v2CgroupRoot, cgroupParent),
		logger:    logger,
		tracker: &cgroupTracker{
			allocToInfo: make(map[string]allocTaskCgroupInfo),
		},
	}
}

type cpusetManagerV2 struct {
	ctx       context.Context
	logger    hclog.Logger
	tracker   *cgroupTracker
	parent    string // relative to cgroup root (e.g. "nomad.slice")
	parentAbs string // absolute path (e.g. "/sys/fs/cgroup/nomad.slice")
}

func (c *cpusetManagerV2) Init() error {
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

	info := make(allocTaskCgroupInfo)
	for task, resources := range alloc.AllocatedResources.Tasks {
		taskCpuset := cpuset.New(resources.Cpu.ReservedCores...)
		// todo, uh what about all this
		// absPath := filepath.Join(c.parentAbs, v2SharedCgroup)
		// relPath := filepath.Join(c.parent, v2SharedCgroup)
		// if !taskCpuset.Empty() {
		//   absPath, relPath = c.pathsForTask(alloc.ID, task)
		// }
		id := CgroupID(alloc.ID, task)
		absPath := filepath.Join(c.parentAbs, id)
		relPath := filepath.Join(c.parent, id)
		info[task] = &TaskCgroupInfo{
			CgroupPath:         absPath,
			RelativeCgroupPath: relPath,
			Cpuset:             taskCpuset,
		}
	}

	c.tracker.set(alloc.ID, info)

	// todo: reconcile
	c.reconcile()
}

func (c cpusetManagerV2) pathsForTask(allocID, task string) (string, string) {
	name := fmt.Sprintf("%s-%s", allocID, task)
	absPath := filepath.Join(c.parentAbs, v2ReservedCgroup, name)
	relPath := filepath.Join(c.parent, v2ReservedCgroup, name)
	return absPath, relPath
}

func (c *cpusetManagerV2) RemoveAlloc(allocID string) {
	c.logger.Trace("remove allocation", "id", allocID)
	c.tracker.delete(allocID)
}

func (c *cpusetManagerV2) CgroupPathFor(allocID, task string) CgroupPathGetter {
	c.logger.Trace("cgroup path for", "id", allocID, "task", task)
	fmt.Println("CPF want CgroupPathFor id:", allocID, "task", task)
	return func(ctx context.Context) (string, error) {
		ticker, cancel := helper.NewSafeTimer(100 * time.Millisecond)
		defer cancel()

		for {
			taskInfo := c.tracker.get(allocID, task)
			if taskInfo == nil {
				fmt.Println("CPF task info is nil for id:", allocID, "task:", task)
				return "", fmt.Errorf("cgroup not found for %s-%s", allocID, task)
			}

			if taskInfo.Error != nil {
				fmt.Println("CPF task info error:", taskInfo.Error)
				return taskInfo.CgroupPath, taskInfo.Error
			}

			mgr, err := fs2.NewManager(nil, taskInfo.CgroupPath, v2isRootless)
			if err != nil {
				fmt.Println("CPF new manager error:", err)
				return "", err
			}

			if mgr.Exists() {
				fmt.Println("CPF exists! id:", allocID, "task:", task, "path:", taskInfo.CgroupPath)
				return taskInfo.CgroupPath, nil
			}

			select {
			case <-ctx.Done():
				fmt.Println("CPF context done, id:", allocID, "task:", task, "error:", ctx.Err())
				return taskInfo.CgroupPath, ctx.Err()
			case <-ticker.C:
				continue
			}
		}
	}
}

func (c *cpusetManagerV2) reconcile() {
	c.tracker.walk(func(info *TaskCgroupInfo) {
		mgr, err := fs2.NewManager(nil, info.CgroupPath, v2isRootless)
		if err != nil {
			panic(err)
		}
		if err = mgr.Apply(v2CreationPID); err != nil {
			panic(err)
		}
	})
}

func (c *cpusetManagerV2) getParent() string {
	return v2Root(c.parent)
}

func (c *cpusetManagerV2) initHierarchy() error {
	// Create parent cgroup

	fmt.Println("SH create:", c.parentAbs)
	parentMgr, parentErr := fs2.NewManager(nil, c.parentAbs, v2isRootless)
	if parentErr != nil {
		return parentErr
	}

	if applyParentErr := parentMgr.Apply(v2CreationPID); applyParentErr != nil {
		return applyParentErr
	}
	fmt.Println("SH created parent")

	c.logger.Debug("established initial cgroup hierarchy", "parent", c.getParent())
	return nil
}

func v2Root(group string) string {
	return filepath.Join(v2CgroupRoot, group)
}

func v2GetCPUsFromCgroup(group string) ([]uint16, error) {
	path := v2Root(group)
	effective, err := cgroups.ReadFile(path, "cpuset.cpus.effective")
	if err != nil {
		return nil, err
	}
	set, err := cpuset.Parse(effective)
	if err != nil {
		return nil, err
	}
	return set.ToSlice(), nil
}

func v2GetParent(parent string) string {
	if parent == "" {
		return V2defaultCgroupParent
	}
	return parent
}

type cgroupTracker struct {
	lock        sync.RWMutex
	allocToInfo map[string]allocTaskCgroupInfo
}

func (cm *cgroupTracker) set(allocID string, info allocTaskCgroupInfo) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.allocToInfo[allocID] = info
}

func (cm *cgroupTracker) delete(allocID string) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	delete(cm.allocToInfo, allocID)
}

func (cm *cgroupTracker) get(allocID, task string) *TaskCgroupInfo {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	allocInfo, allocExists := cm.allocToInfo[allocID]
	if !allocExists {
		return nil
	}
	taskInfo, taskExists := allocInfo[task]
	if !taskExists {
		return nil
	}
	return &TaskCgroupInfo{
		CgroupPath:         taskInfo.CgroupPath,
		RelativeCgroupPath: taskInfo.RelativeCgroupPath,
		Cpuset:             taskInfo.Cpuset.Copy(),
		Error:              taskInfo.Error,
	}
}

func (cm *cgroupTracker) walk(f func(info *TaskCgroupInfo)) {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	for alloc, taskInfo := range cm.allocToInfo {
		for task, info := range taskInfo {
			fmt.Println("walk, alloc:", alloc, "task:", task, "path:", info.CgroupPath)
			f(info)
		}
	}
}
