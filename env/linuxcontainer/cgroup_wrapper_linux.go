package linuxcontainer

import (
	"time"

	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/pkg/cgroup"
)

var (
	_ Cgroup = &wCgroup{}
)

type wCgroup struct {
	cg        *cgroup.Cgroup
	cfsPeriod time.Duration
}

func (c *wCgroup) SetCPURate(s uint64) error {
	if err := c.cg.SetCPUCfsPeriod(uint64(c.cfsPeriod.Microseconds())); err != nil {
		return err
	}
	quota := time.Duration(uint64(c.cfsPeriod) * s / 1000)
	return c.cg.SetCPUCfsQuota(uint64(quota.Microseconds()))
}

func (c *wCgroup) SetCpuset(s string) error {
	return c.cg.SetCpusetCpus([]byte(s))
}

func (c *wCgroup) SetMemoryLimit(s envexec.Size) error {
	return c.cg.SetMemoryLimitInBytes(uint64(s))
}

func (c *wCgroup) SetProcLimit(l uint64) error {
	return c.cg.SetPidsMax(l)
}

func (c *wCgroup) CPUUsage() (time.Duration, error) {
	t, err := c.cg.CpuacctUsage()
	return time.Duration(t), err
}

func (c *wCgroup) MemoryUsage() (envexec.Size, error) {
	s, err := c.cg.MemoryMaxUsageInBytes()
	if err != nil {
		return 0, err
	}
	return envexec.Size(s), nil
	// not really useful if creates new
	// cache, err := (*cgroup.CGroup)(c).FindMemoryStatProperty("cache")
	// if err != nil {
	// 	return 0, err
	// }
	// return envexec.Size(s - cache), err
}

func (c *wCgroup) AddProc(pid int) error {
	return c.cg.AddProc(pid)
}

func (c *wCgroup) Reset() error {
	if err := c.cg.SetCpuacctUsage(0); err != nil {
		return err
	}
	if err := c.cg.SetMemoryMaxUsageInBytes(0); err != nil {
		return err
	}
	return nil
}

func (c *wCgroup) Destroy() error {
	return c.cg.Destroy()
}
