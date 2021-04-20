// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package admin

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPerformance(t *testing.T) {
	dir := t.TempDir()
	prefix := path.Join(dir, "test_")

	p := NewPerformanceService(prefix)

	// Test Start and Stop CPU Profiler
	err := p.StartCPUProfiler()
	assert.NoError(t, err)

	err = p.StopCPUProfiler()
	assert.NoError(t, err)

	_, err = os.Stat(prefix + cpuProfileFile)
	assert.NoError(t, err)

	// Test Stop CPU Profiler without it running
	err = p.StopCPUProfiler()
	assert.Error(t, err)

	// Test Memory Profiler
	err = p.MemoryProfile()
	assert.NoError(t, err)

	_, err = os.Stat(prefix + memProfileFile)
	assert.NoError(t, err)

	// Test Lock Profiler
	err = p.LockProfile()
	assert.NoError(t, err)

	_, err = os.Stat(prefix + lockProfileFile)
	assert.NoError(t, err)
}
