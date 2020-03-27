package pool

import "github.com/criyle/go-judge/pkg/envexec"

var _ envexec.CgroupPool = &FakeCgroupPool{}

// FakeCgroupPool implements cgroup pool but not actually do pool
type FakeCgroupPool struct {
	builder CgroupBuilder
}

// NewFakeCgroupPool creates FakeCgroupPool
func NewFakeCgroupPool(builder CgroupBuilder) *FakeCgroupPool {
	return &FakeCgroupPool{builder: builder}
}

// Get gets new cgroup
func (f *FakeCgroupPool) Get() (envexec.Cgroup, error) {
	cg, err := f.builder.Build()
	if err != nil {
		return nil, err
	}
	return (*wCgroup)(cg), nil
}

// Put destory the cgroup
func (f *FakeCgroupPool) Put(c envexec.Cgroup) {
	c.Destroy()
}

// Shutdown noop
func (f *FakeCgroupPool) Shutdown() {

}
