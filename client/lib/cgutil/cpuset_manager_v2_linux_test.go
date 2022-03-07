package cgutil

import (
	"testing"

	"github.com/hashicorp/nomad/client/testutil"
)

//func Test_identifierRe(t *testing.T) {
//	t.Parallel()
//
//	try := func(name string, exp bool) {
//		result := identifierRe.MatchString(name)
//		require.Equal(t, exp, result)
//	}
//
//	try("523f6798-8acf-29f5-25a8-2feeb45c89c3.sleep2.scope", true)
//}

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
