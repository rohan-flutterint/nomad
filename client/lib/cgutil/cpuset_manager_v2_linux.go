package cgutil

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/nomad/structs"
)

const (
	DefaultCgroupV2Parent = "nomad.slice"
)

func NewCpusetManagerV2(cgroupParent string, logger hclog.Logger) CpusetManager {
	return &cpusetManagerV2{
		// todo
	}
}

type cpusetManagerV2 struct {
}

func (c cpusetManagerV2) Init() error {
	panic("implement me")
}

func (c cpusetManagerV2) AddAlloc(alloc *structs.Allocation) {
	panic("implement me")
}

func (c cpusetManagerV2) RemoveAlloc(allocID string) {
	panic("implement me")
}

func (c cpusetManagerV2) CgroupPathFor(allocID, taskName string) CgroupPathGetter {
	panic("implement me")
}
