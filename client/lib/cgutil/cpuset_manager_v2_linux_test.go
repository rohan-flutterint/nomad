package cgutil

import (
	"testing"

	"github.com/hashicorp/nomad/client/testutil"
)

func TestCpusetManager_V2_Init(t *testing.T) {
	testutil.CgroupV2Compatible(t)

	t.Skip("TODO")
}

func TestCpusetManager_V2_AddAlloc_single(t *testing.T) {
	testutil.CgroupV2Compatible(t)

	t.Skip("TODO")
}

func TestCpusetManager_V2_AddAlloc_subset(t *testing.T) {
	testutil.CgroupV2Compatible(t)

	t.Skip("TODO 11933")
}

func TestCpusetManager_V2_AddAlloc_all(t *testing.T) {
	testutil.CgroupV2Compatible(t)

	t.Skip("TODO 11933")
}

func TestCpusetManager_V2_RemoveAlloc(t *testing.T) {
	testutil.CgroupV2Compatible(t)

	t.Skip("TODO")
}


