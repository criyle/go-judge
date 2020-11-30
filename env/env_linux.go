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
	"github.com/criyle/go-sandbox/pkg/mount"
)

const (
	containerName  = "executor_server"
	defaultWorkDir = "/w"
)

// NewBuilder build a environment builder
func NewBuilder(c Config) (pool.EnvBuilder, error) {
	root, err := ioutil.TempDir("", "executorserver")
	if err != nil {
		return nil, err
	}
	c.Info("Created tmp dir for container root at:", root)

	var mb *mount.Builder
	mc, err := readMountConfig(c.MountConf)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		c.Info("Mount.yaml(", c.MountConf, ") does not exists, use the default container mount")
		mb = getDefaultMount(c.TmpFsParam)
	} else {
		mb, err = parseMountConfig(mc)
		if err != nil {
			return nil, err
		}
	}
	m := mb.FilterNotExist().Mounts
	c.Info("Created container mount at:", mb)

	unshareFlags := uintptr(forkexec.UnshareFlags)
	if c.NetShare {
		unshareFlags ^= syscall.CLONE_NEWNET
	}

	// use setuid container only if running in root privilege
	var credGen container.CredGenerator
	if os.Getuid() == 0 {
		credGen = newCredGen()
	}

	hostName := containerName
	domainName := containerName
	workDir := defaultWorkDir
	if mc != nil {
		hostName = mc.HostName
		domainName = mc.DomainName
		workDir = mc.WorkDir
	}
	c.Info("Creating container builder: hostName=", hostName, ", domainName=", domainName, ", workDir=", workDir)

	b := &container.Builder{
		Root:          root,
		Mounts:        m,
		CredGenerator: credGen,
		Stderr:        os.Stderr,
		CloneFlags:    unshareFlags,
		ExecFile:      c.ContainerInitPath,
		HostName:      hostName,
		DomainName:    domainName,
		WorkDir:       workDir,
	}
	cgb, err := cgroup.NewBuilder(c.CgroupPrefix).WithCPUAcct().WithMemory().WithPids().FilterByEnv()
	if err != nil {
		return nil, err
	}
	c.Info("Test created cgroup builder with:", cgb)
	if cg, err := cgb.Build(); err != nil {
		c.Warn("Tested created cgroup with error", err)
		c.Warn("Failed back to rlimit / rusage mode")
		cgb = nil
	} else {
		cg.Destroy()
	}

	var cgroupPool pool.CgroupPool
	if cgb != nil {
		cgroupPool = pool.NewFakeCgroupPool(cgb)
	}
	return pool.NewEnvBuilder(b, cgroupPool, workDir), nil
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
