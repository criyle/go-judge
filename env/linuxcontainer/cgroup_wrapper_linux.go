package linuxcontainer

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"golang.org/x/sys/unix"
)

var (
	_ Cgroup = &wCgroup{}
)

type wCgroup struct {
	cg        cgroup.Cgroup
	cfsPeriod time.Duration
}

func (c *wCgroup) SetCPURate(s uint64) error {
	quota := time.Duration(uint64(c.cfsPeriod) * s / 1000)
	return c.cg.SetCPUBandwidth(uint64(quota.Microseconds()), uint64(c.cfsPeriod.Microseconds()))
}

func (c *wCgroup) SetCpuset(s string) error {
	return c.cg.SetCPUSet([]byte(s))
}

func (c *wCgroup) SetMemoryLimit(s envexec.Size) error {
	return c.cg.SetMemoryLimit(uint64(s))
}

func (c *wCgroup) SetProcLimit(l uint64) error {
	return c.cg.SetProcLimit(l)
}

func (c *wCgroup) CPUUsage() (time.Duration, error) {
	t, err := c.cg.CPUUsage()
	return time.Duration(t), err
}

func (c *wCgroup) CurrentMemory() (envexec.Size, error) {
	s, err := c.cg.MemoryUsage()
	return envexec.Size(s), err
}

func (c *wCgroup) MaxMemory() (envexec.Size, error) {
	s, err := c.cg.MemoryMaxUsage()
	return envexec.Size(s), err
}

func (c *wCgroup) ProcPeak() (uint64, error) {
	return c.cg.ProcessPeak()
}

func (c *wCgroup) Freeze() error {
	return c.setFrozen(true)
}

func (c *wCgroup) Resume() error {
	return c.setFrozen(false)
}

func (c *wCgroup) setFrozen(frozen bool) error {
	if _, ok := c.cg.(*cgroup.V2); !ok {
		return fmt.Errorf("cgroup freeze: cgroup v2 is required")
	}
	dir, err := c.cg.Open()
	if err != nil {
		return fmt.Errorf("cgroup freeze: open directory: %w", err)
	}
	defer dir.Close()

	want := "0"
	if frozen {
		want = "1"
	}
	if err := writeCgroupAt(int(dir.Fd()), "cgroup.freeze", []byte(want)); err != nil {
		return fmt.Errorf("cgroup freeze: write state: %w", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		state, err := readCgroupAt(int(dir.Fd()), "cgroup.events")
		if err != nil {
			return fmt.Errorf("cgroup freeze: read state: %w", err)
		}
		if cgroupEventValue(state, "frozen") == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("cgroup freeze: state did not become %s: %w", want, os.ErrDeadlineExceeded)
		}
		time.Sleep(time.Millisecond)
	}
}

func writeCgroupAt(dirfd int, name string, content []byte) error {
	fd, err := unix.Openat(dirfd, name, unix.O_WRONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	for len(content) > 0 {
		n, err := unix.Write(fd, content)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		content = content[n:]
	}
	return nil
}

func readCgroupAt(dirfd int, name string) ([]byte, error) {
	fd, err := unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	var b bytes.Buffer
	_, err = b.ReadFrom(f)
	return b.Bytes(), err
}

func cgroupEventValue(content []byte, name string) string {
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == name {
			if _, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				return fields[1]
			}
		}
	}
	return ""
}

func (c *wCgroup) AddProc(pid int) error {
	return c.cg.AddProc(pid)
}

func (c *wCgroup) Reset() error {
	if _, ok := c.cg.(*cgroup.V2); ok {
		return c.Resume()
	}
	return nil
}

func (c *wCgroup) Destroy() error {
	return c.cg.Destroy()
}

func (c *wCgroup) Open() (*os.File, error) {
	return c.cg.Open()
}
