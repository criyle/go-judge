package linuxcontainer

import "time"

var _ CgroupPool = &FakeCgroupPool{}

// FakeCgroupPool implements cgroup pool but not actually do pool
type FakeCgroupPool struct {
	builder   CgroupBuilder
	cfsPeriod time.Duration
}

// NewFakeCgroupPool creates FakeCgroupPool
func NewFakeCgroupPool(builder CgroupBuilder, cfsPeriod time.Duration) CgroupPool {
	return &FakeCgroupPool{builder: builder, cfsPeriod: cfsPeriod}
}

// Get gets new cgroup
func (f *FakeCgroupPool) Get() (Cgroup, error) {
	cg, err := f.builder.Random("")
	if err != nil {
		return nil, err
	}
	return &wCgroup{cg: cg, cfsPeriod: f.cfsPeriod}, nil
}

// Put destroy the cgroup
func (f *FakeCgroupPool) Put(c Cgroup) {
	c.Destroy()
}

// Shutdown noop
func (f *FakeCgroupPool) Shutdown() {

}
