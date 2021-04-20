// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package admin

import (
	"errors"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/ava-labs/avalanchego/utils/perms"
)

const (
	// Name of file that CPU profile is written to when StartCPUProfiler called
	cpuProfileFile = "cpu.profile"
	// Name of file that memory profile is written to when MemoryProfile called
	memProfileFile = "mem.profile"
	// Name of file that lock profile is written to
	lockProfileFile = "lock.profile"
)

var (
	errCPUProfilerRunning    = errors.New("cpu profiler already running")
	errCPUProfilerNotRunning = errors.New("cpu profiler doesn't exist")
)

// Performance provides helper methods for measuring the current performance of
// the system
type Performance struct {
	cpuProfileName, memProfileName, lockProfileName string
	cpuProfileFile                                  *os.File
}

func NewPerformanceService(prefix string) *Performance {
	return &Performance{
		cpuProfileName:  prefix + cpuProfileFile,
		memProfileName:  prefix + memProfileFile,
		lockProfileName: prefix + lockProfileFile,
	}
}

func NewDefaultPerformanceService() *Performance { return NewPerformanceService("") }

// StartCPUProfiler starts measuring the cpu utilization of this node
func (p *Performance) StartCPUProfiler() error {
	if p.cpuProfileFile != nil {
		return errCPUProfilerRunning
	}

	file, err := perms.Create(p.cpuProfileName, perms.ReadWrite)
	if err != nil {
		return err
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		_ = file.Close() // Return the original error
		return err
	}
	runtime.SetMutexProfileFraction(1)

	p.cpuProfileFile = file
	return nil
}

// StopCPUProfiler stops measuring the cpu utilization of this node
func (p *Performance) StopCPUProfiler() error {
	if p.cpuProfileFile == nil {
		return errCPUProfilerNotRunning
	}

	pprof.StopCPUProfile()
	err := p.cpuProfileFile.Close()
	p.cpuProfileFile = nil
	return err
}

// MemoryProfile dumps the current memory utilization of this node
func (p *Performance) MemoryProfile() error {
	file, err := perms.Create(p.memProfileName, perms.ReadWrite)
	if err != nil {
		return err
	}
	runtime.GC() // get up-to-date statistics
	if err := pprof.WriteHeapProfile(file); err != nil {
		_ = file.Close() // Return the original error
		return err
	}
	return file.Close()
}

// LockProfile dumps the current lock statistics of this node
func (p *Performance) LockProfile() error {
	file, err := perms.Create(p.lockProfileName, perms.ReadWrite)
	if err != nil {
		return err
	}

	profile := pprof.Lookup("mutex")
	if err := profile.WriteTo(file, 1); err != nil {
		_ = file.Close() // Return the original error
		return err
	}
	return file.Close()
}
