package cgutil

import (
	"context"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/helper"
	"github.com/hashicorp/nomad/lib/cpuset"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/cgroups/fs2"
	"github.com/opencontainers/runc/libcontainer/configs"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

// identifier is the "<allocID>.<taskName>" string that uniquely identifies an
// individual instance of a task within the flat cgroup namespace
type identifier = string

// nothing is used for treating a map like a set with no values
type nothing struct{}

var null = nothing{}

type cpusetManagerV2 struct {
	ctx    context.Context
	logger hclog.Logger

	parent    string        // relative to cgroup root (e.g. "nomad.slice")
	parentAbs string        // absolute path (e.g. "/sys/fs/cgroup/nomad.slice")
	initial   cpuset.CPUSet // set of initial cores (never changes)

	lock      sync.RWMutex                 // hold this with regard to tracking fields
	pool      cpuset.CPUSet                // cores being shared among across all tasks
	sharing   map[identifier]nothing       // sharing tasks which use only cpus in the pool
	isolating map[identifier]cpuset.CPUSet // remember which task goes where to avoid context switching
}

func NewCpusetManagerV2(parent string, logger hclog.Logger) CpusetManager {
	cgroupParent := v2GetParent(parent)
	return &cpusetManagerV2{
		ctx:       context.TODO(),
		parent:    cgroupParent,
		parentAbs: filepath.Join(v2CgroupRoot, cgroupParent),
		logger:    logger,
		sharing:   make(map[identifier]nothing),
		isolating: make(map[identifier]cpuset.CPUSet),
	}
}

func (c *cpusetManagerV2) Init(cores []uint16) error {
	c.logger.Warn("initializing with", "cores", cores)
	if err := c.ensureParent(); err != nil {
		c.logger.Error("failed to init cpuset manager", "err", err)
		return err
	}
	c.initial = cpuset.New(cores...)
	return nil
}

func (c *cpusetManagerV2) AddAlloc(alloc *structs.Allocation) {
	if alloc == nil || alloc.AllocatedResources == nil {
		return
	}

	c.logger.Info("add allocation", "name", alloc.Name, "id", alloc.ID)

	c.lock.Lock()
	// defer c.lock.Unlock()

	// first update our tracking of isolating and sharing tasks
	for task, resources := range alloc.AllocatedResources.Tasks {
		id := makeID(alloc.ID, task)
		if len(resources.Cpu.ReservedCores) > 0 {
			c.isolating[id] = cpuset.New(resources.Cpu.ReservedCores...)
			fmt.Println(" isolating id:", id, "cores:", c.isolating[id])
		} else {
			c.sharing[id] = null
			fmt.Println(" sharing id:", id)
		}
	}

	// recompute the available sharable cpu cores
	c.recalculate()
	fmt.Println(" remaining:", c.pool)

	// todo remove
	c.lock.Unlock()

	// now write out the entire cgroups space
	// todo in background
	c.reconcile()
}

func (c *cpusetManagerV2) RemoveAlloc(allocID string) {
	c.logger.Info("remove allocation", "id", allocID)

	c.lock.Lock()

	// remove tasks of allocID from the sharing set
	for id := range c.sharing {
		if strings.HasPrefix(id, allocID) {
			delete(c.sharing, id)
		}
	}

	// remove tasks of allocID from the isolating set
	for id := range c.isolating {
		if strings.HasPrefix(id, allocID) {
			delete(c.isolating, id)
		}
	}

	// recompute available sharable cpu cores
	c.recalculate()
	fmt.Println(" remaining:", c.pool)

	// todo move
	c.lock.Unlock()

	// now write out the entire cgroups space
	// todo in background
	c.reconcile()
}

// recalculate the number of cores sharable by non-isolating tasks (and isolating tasks)
func (c *cpusetManagerV2) recalculate() {
	remaining := c.initial.Copy()
	for _, set := range c.isolating {
		remaining = remaining.Difference(set)
	}
	c.pool = remaining
}

func (c *cpusetManagerV2) CgroupPathFor(allocID, task string) CgroupPathGetter {
	c.logger.Info("cgroup path for", "id", allocID, "task", task)

	// block until cgroup for allocID.task exists

	return func(ctx context.Context) (string, error) {
		ticks, cancel := helper.NewSafeTimer(100 * time.Millisecond)
		defer cancel()

		for {
			path := c.pathOf(makeID(allocID, task))
			mgr, err := fs2.NewManager(nil, path, v2isRootless)
			if err != nil {
				return "", err
			}

			if mgr.Exists() {
				return path, nil
			}

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-ticks.C:
				continue
			}
		}
	}
}

func (c *cpusetManagerV2) reconcile() {
	c.lock.RLock()
	defer c.lock.RUnlock()

	fmt.Println("reconcile ... ")

	for id := range c.sharing {
		fmt.Println(" sharing id:", id)
		c.write(id, c.pool)
	}

	for id, set := range c.isolating {
		fmt.Println(" isolating id:", id, "set:", set)
		c.write(id, c.pool.Union(set))
	}
}

func (c *cpusetManagerV2) pathOf(id string) string {
	return filepath.Join(c.parentAbs, makeScope(id))
}

func (c *cpusetManagerV2) write(id string, set cpuset.CPUSet) {
	path := c.pathOf(id)

	// make a manager for the cgroup
	m, err := fs2.NewManager(nil, path, v2isRootless)
	if err != nil {
		c.logger.Error("failed to manage cgroup", "path", path, "err", err)
	}

	// create the cgroup
	if err = m.Apply(v2CreationPID); err != nil {
		c.logger.Error("failed to apply cgroup", "path", path, "err", err)
	}

	// set the cpuset value for the cgroup
	if err = m.Set(&configs.Resources{
		CpusetCpus: set.String(),
	}); err != nil {
		c.logger.Error("failed to set cgroup", "path", path, "err", err)
	}
}

func (c *cpusetManagerV2) getParent() string {
	return v2Root(c.parent)
}

// ensureParentCgroup will create parent cgroup for the manager if it does not
// exist yet. No PIDs are added to any cgroup yet.
func (c *cpusetManagerV2) ensureParent() error {
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

// identifierRe matches one of our managed cgroups
// var identifierRe = regexp.MustCompile(`\.scope$`)

// not necessary, AddAlloc is called again on startup
//func (c *cpusetManagerV2) restore() error {
//	fmt.Println("SH will restore ...")
//	return filepath.Walk(c.parentAbs, func(path string, info os.FileInfo, err error) error {
//		// cgroup must be a directory
//		if !info.IsDir() {
//			return nil
//		}
//
//		// cgroup must be under parent (v2 is using flat namespace)
//		if dir := filepath.Dir(path); dir != c.parentAbs {
//			return nil
//		}
//
//		// cgroup must match naming pattern
//		name := filepath.Base(path)
//		matches := identifierRe.MatchString(name)
//		if matches {
//			fmt.Println("restore cgroup:", path)
//			// actually read things
//			fm, readErr := fs2.NewManager(nil, path, v2isRootless)
//			if readErr != nil {
//				return readErr
//			}
//			groups, groupErr := fm.GetCgroups()
//			if groupErr != nil {
//				return groupErr
//			}
//			cpus := groups.CpusetCpus
//			fmt.Println(" -> cpus:", cpus)
//		}
//		return nil
//	})
//}

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
