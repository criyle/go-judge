package env

import (
	"io/ioutil"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/criyle/go-judge/pkg/pool"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/forkexec"
)

// NewBuilder build a environment builder
func NewBuilder(cinitPath, mountConf, tmpFsConf string, netShare bool, printLog func(v ...interface{})) (pool.EnvBuilder, error) {
	root, err := ioutil.TempDir("", "executorserver")
	if err != nil {
		return nil, err
	}
	printLog("Created tmp dir for container root at:", root)

	mb, err := parseMountConfig(mountConf)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		printLog("Use the default container mount")
		mb = getDefaultMount(tmpFsConf)
	}
	m, err := mb.Build(true)
	if err != nil {
		return nil, err
	}
	printLog("Created container mount at:", mb)

	unshareFlags := uintptr(forkexec.UnshareFlags)
	if netShare {
		unshareFlags ^= syscall.CLONE_NEWNET
	}

	// use setuid container only if running in root privilege
	var credGen container.CredGenerator
	if os.Getuid() == 0 {
		credGen = newCredGen()
	}

	b := &container.Builder{
		Root:          root,
		Mounts:        m,
		CredGenerator: credGen,
		Stderr:        true,
		CloneFlags:    unshareFlags,
		ExecFile:      cinitPath,
	}
	cgb, err := cgroup.NewBuilder("executorserver").WithCPUAcct().WithMemory().WithPids().FilterByEnv()
	if err != nil {
		return nil, err
	}
	printLog("Created cgroup builder with:", cgb)
	if cg, err := cgb.Build(); err != nil {
		printLog("Tested created cgroup with error", err)
		printLog("Failed back to rlimit / rusage mode")
		cgb = nil
	} else {
		cg.Destroy()
	}

	var cgroupPool pool.CgroupPool
	if cgb != nil {
		cgroupPool = pool.NewFakeCgroupPool(cgb)
	}
	return pool.NewEnvBuilder(b, cgroupPool), nil
}

type credGen struct {
	cur uint32
}

func newCredGen() *credGen {
	return &credGen{cur: 10000}
}

func (c *credGen) Get() syscall.Credential {
	n := atomic.AddUint32(&c.cur, 1)
	return syscall.Credential{
		Uid: n,
		Gid: n,
	}
}
