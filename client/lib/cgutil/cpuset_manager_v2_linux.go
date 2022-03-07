package cgutil

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
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
	}
}

// identifier is the "<allocID>.<taskName>" string that uniquely identifies an
// individual instance of a task within the flat cgroup namespace
type identifier = string

type cpusetManagerV2 struct {
	ctx       context.Context
	logger    hclog.Logger
	parent    string // relative to cgroup root (e.g. "nomad.slice")
	parentAbs string // absolute path (e.g. "/sys/fs/cgroup/nomad.slice")

	pool      cpuset.CPUSet                // either detected or configured via client config
	sharing   map[identifier]struct{}      // tasks using only cpus in the pool
	isolating map[identifier]cpuset.CPUSet // remember which task goes where to avoid context switching
}

func (c *cpusetManagerV2) Init(cores []uint16) error {
	c.logger.Info("initializing with", "cores", cores)
	if err := c.ensureParentExists(); err != nil {
		c.logger.Error("failed to init cpuset manager", "err", err)
		return err
	}
	c.sharing = cpuset.New(cores...)
	c.isolating = make(map[identifier]cpuset.CPUSet)
	return nil
}

func (c *cpusetManagerV2) AddAlloc(alloc *structs.Allocation) {
	if alloc == nil || alloc.AllocatedResources == nil {
		return
	}
	c.logger.Trace("add allocation", "name", alloc.Name, "id", alloc.ID)

	for task, resources := range alloc.AllocatedResources.Tasks {
		fmt.Println("AddAlloc task:", task, "resources:", resources.Cpu.ReservedCores)
	}

	// TODO: YOU ARE HERE
	// trying to do bookkeeping without depending on the filesystem (or cgroup inheretence)
	// found docker driver lets you specify cgroup, so should be fine, we create it here
	//  - figure out how to plumb cpuset through

	// rebuild the flat tree every time (need to re-arrange all cpusets)

	// set the latest build on c

	// let the reconciler use the latest available

	//previous := c.tracker.current()
	//
	//for task, resources := range alloc.AllocatedResources.Tasks {
	//	reservedSet := cpuset.New(resources.Cpu.ReservedCores...)
	//	absPath := filepath.Join(c.parentAbs, makeID(alloc.ID, task))
	//	info := TaskCgroupInfo{
	//		AllocID:            "",
	//		Task:               "",
	//		CgroupPath:         "",
	//		RelativeCgroupPath: "",
	//		Cpuset:             cpuset.CPUSet{},
	//		Error:              nil,
	//	}
	//}

	//info := make(allocTaskCgroupInfo)
	//for task, resources := range alloc.AllocatedResources.Tasks {
	//	taskCpuset := cpuset.New(resources.Cpu.ReservedCores...)
	//	// todo, uh what about all this
	//	// absPath := filepath.Join(c.parentAbs, v2SharedCgroup)
	//	// relPath := filepath.Join(c.parent, v2SharedCgroup)
	//	// if !taskCpuset.Empty() {
	//	//   absPath, relPath = c.pathsForTask(alloc.ID, task)
	//	// }
	//	id := CgroupID(alloc.ID, task)
	//	absPath := filepath.Join(c.parentAbs, id)
	//	relPath := filepath.Join(c.parent, id)
	//	info[task] = &TaskCgroupInfo{
	//		CgroupPath:         absPath,
	//		RelativeCgroupPath: relPath,
	//		Cpuset:             taskCpuset,
	//	}
	//}
	//
	//c.tracker.set(alloc.ID, info)

	// todo: reconcile
	c.reconcile()
}

func (c *cpusetManagerV2) RemoveAlloc(allocID string) {
	c.logger.Trace("remove allocation", "id", allocID)
	// c.tracker.delete(allocID)
}

func (c *cpusetManagerV2) CgroupPathFor(allocID, task string) CgroupPathGetter {
	c.logger.Trace("cgroup path for", "id", allocID, "task", task)
	//fmt.Println("CPF want CgroupPathFor id:", allocID, "task", task)
	//return func(ctx context.Context) (string, error) {
	//	ticker, cancel := helper.NewSafeTimer(100 * time.Millisecond)
	//	defer cancel()
	//
	//	for {
	//		taskInfo := c.tracker.get(allocID, task)
	//		if taskInfo.AllocID == "" || taskInfo.Task == "" {
	//			fmt.Println("CPF task info was missing for id:", allocID, "task:", task)
	//		}
	//
	//		if taskInfo.Error != nil {
	//			fmt.Println("CPF task info error:", taskInfo.Error)
	//			return taskInfo.CgroupPath, taskInfo.Error
	//		}
	//
	//		mgr, err := fs2.NewManager(nil, taskInfo.CgroupPath, v2isRootless)
	//		if err != nil {
	//			fmt.Println("CPF new manager error:", err)
	//			return "", err
	//		}
	//
	//		if mgr.Exists() {
	//			fmt.Println("CPF exists! id:", allocID, "task:", task, "path:", taskInfo.CgroupPath)
	//			return taskInfo.CgroupPath, nil
	//		}
	//
	//		select {
	//		case <-ctx.Done():
	//			fmt.Println("CPF context done, id:", allocID, "task:", task, "error:", ctx.Err())
	//			return taskInfo.CgroupPath, ctx.Err()
	//		case <-ticker.C:
	//			continue
	//		}
	//	}
	//}
	return nil
}

func (c *cpusetManagerV2) reconcile() {
	//c.tracker.apply(func(info TaskCgroupInfo) {
	//	mgr, err := fs2.NewManager(nil, info.CgroupPath, v2isRootless)
	//	if err != nil {
	//		panic(err)
	//	}
	//	if err = mgr.Apply(v2CreationPID); err != nil {
	//		panic(err)
	//	}
	//})
}

func (c *cpusetManagerV2) getParent() string {
	return v2Root(c.parent)
}

// ensureParentCgroup will create parent cgroup for the manager if it does not
// exist yet. No PIDs are added to any cgroup yet.
func (c *cpusetManagerV2) ensureParentExists() error {
	parentMgr, parentErr := fs2.NewManager(nil, c.parentAbs, v2isRootless)
	if parentErr != nil {
		return parentErr
	}

	if applyParentErr := parentMgr.Apply(v2CreationPID); applyParentErr != nil {
		return applyParentErr
	}

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

//
//type tracker struct {
//	lock  sync.RWMutex
//	infos map[string]TaskCgroupInfo // "allocID+task" -> cgroup
//}
//
//func (t *tracker) set(infos []TaskCgroupInfo) {
//	m := make(map[string]TaskCgroupInfo)
//	for _, info := range infos {
//		m[info.ID()] = info
//	}
//
//	t.lock.Lock()
//	defer t.lock.Unlock()
//
//	t.infos = m
//}
//
//func (t *tracker) get(allocID, task string) TaskCgroupInfo {
//	t.lock.RLock()
//	defer t.lock.RUnlock()
//
//	return t.infos[makeID(allocID, task)]
//}
//
//func (t *tracker) current() map[string]TaskCgroupInfo {
//	t.lock.RLock()
//	defer t.lock.RUnlock()
//
//	m := make(map[string]TaskCgroupInfo)
//	for k, v := range t.infos {
//		m[k] = v
//	}
//	return m
//}
//
//func (t *tracker) apply(f func(i TaskCgroupInfo)) {
//	t.lock.Lock()
//	defer t.lock.Unlock()
//
//	for _, info := range t.infos {
//		f(info)
//	}
//}
