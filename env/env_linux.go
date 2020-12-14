package env

import (
	"fmt"
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
	containerName      = "executor_server"
	defaultWorkDir     = "/w"
	containerCredStart = 10000
	containerCred      = 1000
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

	seccomp, err := readSeccompConf(c.SeccompConf)
	if err != nil {
		return nil, fmt.Errorf("failed to load seccomp config: %v", err)
	}
	if seccomp != nil {
		c.Info("Load seccomp filter: ", c.SeccompConf)
	}

	unshareFlags := uintptr(forkexec.UnshareFlags)
	if c.NetShare {
		unshareFlags ^= syscall.CLONE_NEWNET
	}

	// use setuid container only if running in root privilege
	var credGen container.CredGenerator
	if os.Getuid() == 0 {
		cred := c.ContainerCredStart
		if cred == 0 {
			cred = containerCredStart
		}
		credGen = newCredGen(uint32(cred))
	}

	hostName := containerName
	domainName := containerName
	workDir := defaultWorkDir
	cUID := containerCred
	cGID := containerCred
	if mc != nil {
		hostName = mc.HostName
		domainName = mc.DomainName
		workDir = mc.WorkDir
		cUID = mc.UID
		cGID = mc.GID
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
		ContainerUID:  cUID,
		ContainerGID:  cGID,
	}
	cgb := cgroup.NewBuilder(c.CgroupPrefix).WithCPUAcct().WithMemory().WithPids()
	if c.Cpuset != "" {
		cgb = cgb.WithCPUSet()
	}
	if c.EnableCPURate {
		cgb = cgb.WithCPU()
	}
	cgb, err = cgb.FilterByEnv()
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
		cgroupPool = pool.NewFakeCgroupPool(cgb, c.CPUCfsPeriod)
	}
	return pool.NewEnvBuilder(pool.Config{
		Builder:    b,
		CgroupPool: cgroupPool,
		WorkDir:    workDir,
		Cpuset:     c.Cpuset,
		CPURate:    c.EnableCPURate,
		Seccomp:    seccomp,
	}), nil
}

type credGen struct {
	cur uint32
}

func newCredGen(start uint32) *credGen {
	return &credGen{cur: start}
}

func (c *credGen) Get() syscall.Credential {
	n := atomic.AddUint32(&c.cur, 1)
	return syscall.Credential{
		Uid: n,
		Gid: n,
	}
}
