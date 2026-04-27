//go:build linux

package envexec

import (
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func runWithCPUAffinity(cpuset string, fn func()) {
	if cpuset == "" {
		fn()
		return
	}

	cpuSet, ok := parseCPUSet(cpuset)
	if !ok {
		fn()
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var old unix.CPUSet
	if err := unix.SchedGetaffinity(0, &old); err != nil {
		fn()
		return
	}
	if err := unix.SchedSetaffinity(0, cpuSet); err != nil {
		fn()
		return
	}
	defer unix.SchedSetaffinity(0, &old)

	fn()
}

func parseCPUSet(value string) (*unix.CPUSet, bool) {
	var cpuSet unix.CPUSet
	for _, segment := range strings.Split(value, ",") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		if lo, hi, ok := strings.Cut(segment, "-"); ok {
			start, err := strconv.Atoi(strings.TrimSpace(lo))
			if err != nil {
				return nil, false
			}
			end, err := strconv.Atoi(strings.TrimSpace(hi))
			if err != nil || end < start {
				return nil, false
			}
			for i := start; i <= end; i++ {
				cpuSet.Set(i)
			}
			continue
		}

		cpu, err := strconv.Atoi(segment)
		if err != nil {
			return nil, false
		}
		cpuSet.Set(cpu)
	}
	return &cpuSet, true
}
