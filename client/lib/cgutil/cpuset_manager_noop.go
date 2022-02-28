//go:build !linux
// +build !linux

package cgutil

// NewCpusetManager creates a CpusetManager that does nothing, for operating
// systems that do not support cgroups.
func NewCpusetManager(string, hclog.Logger) CpusetManager {
	return new(NoopCpusetManager)
}
