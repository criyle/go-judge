package env

import (
	"fmt"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/criyle/go-judge/env/linuxcontainer"
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/forkexec"
	"github.com/criyle/go-sandbox/pkg/mount"
	"golang.org/x/sys/unix"
)

const (
	containerName      = "executor_server"
	defaultWorkDir     = "/w"
	containerCredStart = 10000
	containerCred      = 1000
)

// NewBuilder build a environment builder
func NewBuilder(c Config) (pool.EnvBuilder, error) {
	var (
		mountBuilder  *mount.Builder
		symbolicLinks []container.SymbolicLink
		maskPaths     []string
	)
	mc, err := readMountConfig(c.MountConf)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		c.Info("Mount.yaml(", c.MountConf, ") does not exists, use the default container mount")
		mountBuilder = getDefaultMount(c.TmpFsParam)
	} else {
		mountBuilder, err = parseMountConfig(mc)
		if err != nil {
			return nil, err
		}
	}
	if mc != nil && len(mc.SymLinks) > 0 {
		symbolicLinks = make([]container.SymbolicLink, 0, len(mc.SymLinks))
		for _, l := range mc.SymLinks {
			symbolicLinks = append(symbolicLinks, container.SymbolicLink{LinkPath: l.LinkPath, Target: l.Target})
		}
	} else {
		symbolicLinks = defaultSymLinks
	}
	if mc != nil && len(mc.MaskPaths) > 0 {
		maskPaths = mc.MaskPaths
	} else {
		maskPaths = defaultMaskPaths
	}
	m := mountBuilder.FilterNotExist().Mounts
	c.Info("Created container mount at:", mountBuilder)

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
	major, minor := kernelVersion()
	if major < 4 || (major == 4 && minor < 6) {
		unshareFlags ^= unix.CLONE_NEWCGROUP
		c.Info("Kernel version (", major, ".", minor, ") < 4.6, don't unshare cgroup")
	}

	// use setuid container only if running in root privilege
	var credGen container.CredGenerator
	if os.Getuid() == 0 && c.ContainerCredStart > 0 {
		credGen = newCredGen(uint32(c.ContainerCredStart))
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
		TmpRoot:       "executorserver",
		Mounts:        m,
		SymbolicLinks: symbolicLinks,
		MaskPaths:     maskPaths,
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
	t := cgroup.DetectType()
	if t == cgroup.CgroupTypeV2 {
		c.Info("Enable cgroup v2 nesting support")
		if err := cgroup.EnableV2Nesting(); err != nil {
			c.Warn("Enable cgroup v2 failed", err)
		}
	}
	cgb := cgroup.NewBuilder(c.CgroupPrefix).WithType(t).WithCPUAcct().WithMemory().WithPids().WithCPUSet()
	if c.EnableCPURate {
		cgb = cgb.WithCPU()
	}
	cgb, err = cgb.FilterByEnv()
	if err != nil {
		return nil, err
	}
	c.Info("Test created cgroup builder with: ", cgb)
	if cg, err := cgb.Random(""); err != nil {
		c.Warn("Tested created cgroup with error: ", err)
		c.Warn("Failed back to rlimit / rusage mode")
		cgb = nil
	} else {
		cg.Destroy()
	}

	var cgroupPool linuxcontainer.CgroupPool
	if cgb != nil {
		cgroupPool = linuxcontainer.NewFakeCgroupPool(cgb, c.CPUCfsPeriod)
	}
	return linuxcontainer.NewEnvBuilder(linuxcontainer.Config{
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

func kernelVersion() (major int, minor int) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return
	}

	rl := uname.Release
	var values [2]int
	vi := 0
	value := 0
	for _, c := range rl {
		if '0' <= c && c <= '9' {
			value = (value * 10) + int(c-'0')
		} else {
			// Note that we're assuming N.N.N here.  If we see anything else we are likely to
			// mis-parse it.
			values[vi] = value
			vi++
			if vi >= len(values) {
				break
			}
			value = 0
		}
	}
	switch vi {
	case 0:
		return 0, 0
	case 1:
		return values[0], 0
	case 2:
		return values[0], values[1]
	}
	return
}
