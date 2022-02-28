//go:build !linux
// +build !linux

package cgutil

const (
	DefaultCgroupParent = ""
)

// FindCgroupMountpointDir is used to find the cgroup mount point on a Linux
// system. Here the implementation is a no-op.
func FindCgroupMountpointDir() (string, error) {
	return "", nil
}
