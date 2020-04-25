package pool

var _ CgroupPool = &FakeCgroupPool{}

// FakeCgroupPool implements cgroup pool but not actually do pool
type FakeCgroupPool struct {
	builder CgroupBuilder
}

// NewFakeCgroupPool creates FakeCgroupPool
func NewFakeCgroupPool(builder CgroupBuilder) CgroupPool {
	return &FakeCgroupPool{builder: builder}
}

// Get gets new cgroup
func (f *FakeCgroupPool) Get() (Cgroup, error) {
	cg, err := f.builder.Build()
	if err != nil {
		return nil, err
	}
	return (*wCgroup)(cg), nil
}

// Put destory the cgroup
func (f *FakeCgroupPool) Put(c Cgroup) {
	c.Destroy()
}

// Shutdown noop
func (f *FakeCgroupPool) Shutdown() {

}
