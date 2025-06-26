package env

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/criyle/go-judge/env/linuxcontainer"
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/forkexec"
	"github.com/criyle/go-sandbox/pkg/mount"
	"github.com/criyle/go-sandbox/runner"
	"github.com/google/shlex"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	containerName      = "executor_server"
	defaultWorkDir     = "/w"
	containerCredStart = 10000
	containerCred      = 1000
)

// NewBuilder build a environment builder
func NewBuilder(c Config, logger *zap.Logger) (pool.EnvBuilder, map[string]any, error) {
	mountsConfig, mountBuilder, symbolicLinks, maskPaths, err := prepareMountAndPaths(c, logger)
	if err != nil {
		return nil, nil, err
	}
	m := mountBuilder.FilterNotExist().Mounts

	seccomp, err := prepareSeccomp(c, logger)
	if err != nil {
		return nil, nil, err
	}
	unshareFlags, unshareCgroup := prepareUnshareFlags(c, logger)
	credGen := prepareCredGen(c)
	hostName, domainName, workDir, cUID, cGID, initCmd, err := prepareContainerMeta(mountsConfig, logger)
	if err != nil {
		return nil, nil, err
	}

	b := &container.Builder{
		TmpRoot:                 "go-judge",
		Mounts:                  m,
		SymbolicLinks:           symbolicLinks,
		MaskPaths:               maskPaths,
		CredGenerator:           credGen,
		Stderr:                  os.Stderr,
		CloneFlags:              unshareFlags,
		ExecFile:                c.ContainerInitPath,
		HostName:                hostName,
		DomainName:              domainName,
		InitCommand:             initCmd,
		WorkDir:                 workDir,
		ContainerUID:            cUID,
		ContainerGID:            cGID,
		UnshareCgroupBeforeExec: unshareCgroup,
	}

	cgb, ct, err := setupCgroup(c, logger)
	if err != nil {
		return nil, nil, err
	}

	cgroupPool := prepareCgroupPool(cgb, c)
	cgroupType, cgroupControllers := getCgroupInfo(cgb, ct)

	conf := map[string]any{
		"cgroupType":        cgroupType,
		"mount":             m,
		"symbolicLink":      symbolicLinks,
		"maskedPaths":       maskPaths,
		"hostName":          hostName,
		"domainName":        domainName,
		"workDir":           workDir,
		"uid":               cUID,
		"gid":               cGID,
		"cgroupControllers": cgroupControllers,
	}

	if tryClone3Builder := tryClone3(c, b, cgb, cgroupType, cgroupPool, workDir, seccomp, logger); tryClone3Builder != nil {
		conf["clone3"] = true
		return tryClone3Builder, conf, nil
	}

	return linuxcontainer.NewEnvBuilder(linuxcontainer.Config{
		Builder:    b,
		CgroupPool: cgroupPool,
		WorkDir:    workDir,
		Cpuset:     c.Cpuset,
		CPURate:    c.EnableCPURate,
		Seccomp:    seccomp,
	}), conf, nil
}

func prepareMountAndPaths(c Config, logger *zap.Logger) (*Mounts, *mount.Builder, []container.SymbolicLink, []string, error) {
	mc, err := readMountConfig(c.MountConf)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Error("failed to read mount config", zap.String("path", c.MountConf), zap.Error(err))
			return nil, nil, nil, nil, err
		}
		logger.Info("mount.yaml does not exist, using default container mount", zap.String("path", c.MountConf))
		return nil, getDefaultMount(c.TmpFsParam), defaultSymLinks, defaultMaskPaths, nil
	}
	mountBuilder, err := parseMountConfig(mc)
	if err != nil {
		logger.Error("failed to parse mount config", zap.Error(err))
		return nil, nil, nil, nil, err
	}
	var symbolicLinks []container.SymbolicLink
	if len(mc.SymLinks) > 0 {
		symbolicLinks = make([]container.SymbolicLink, 0, len(mc.SymLinks))
		for _, l := range mc.SymLinks {
			symbolicLinks = append(symbolicLinks, container.SymbolicLink{LinkPath: l.LinkPath, Target: l.Target})
		}
	} else {
		symbolicLinks = defaultSymLinks
	}
	maskPaths := defaultMaskPaths
	if len(mc.MaskPaths) > 0 {
		maskPaths = mc.MaskPaths
	}
	logger.Info("created container mount", zap.Any("mountBuilder", mountBuilder))
	return mc, mountBuilder, symbolicLinks, maskPaths, nil
}

func prepareSeccomp(c Config, logger *zap.Logger) ([]syscall.SockFilter, error) {
	seccomp, err := readSeccompConf(c.SeccompConf)
	if err != nil {
		logger.Error("failed to load seccomp config", zap.String("path", c.SeccompConf), zap.Error(err))
		return nil, fmt.Errorf("failed to load seccomp config: %w", err)
	}
	if seccomp != nil {
		logger.Info("loaded seccomp filter", zap.String("path", c.SeccompConf))
	}
	return seccomp, nil
}

func prepareUnshareFlags(c Config, logger *zap.Logger) (uintptr, bool) {
	unshareFlags := uintptr(forkexec.UnshareFlags)
	if c.NetShare {
		unshareFlags ^= syscall.CLONE_NEWNET
	}
	major, minor := kernelVersion()
	unshareFlags ^= unix.CLONE_NEWCGROUP
	unshareCgroup := true
	if major < 4 || (major == 4 && minor < 6) {
		unshareCgroup = false
		logger.Info("kernel version < 4.6, don't unshare cgroup", zap.Int("major", major), zap.Int("minor", minor))
	}
	return unshareFlags, unshareCgroup
}

func prepareCredGen(c Config) container.CredGenerator {
	if os.Getuid() == 0 && c.ContainerCredStart > 0 {
		return newCredGen(uint32(c.ContainerCredStart))
	}
	return nil
}

func prepareContainerMeta(mc *Mounts, logger *zap.Logger) (hostName, domainName, workDir string, cUID, cGID int, initCmd []string, err error) {
	hostName = containerName
	domainName = containerName
	workDir = defaultWorkDir
	cUID = containerCred
	cGID = containerCred
	if mc != nil {
		if mc.HostName != "" {
			hostName = mc.HostName
		}
		if mc.DomainName != "" {
			domainName = mc.DomainName
		}
		if mc.WorkDir != "" {
			workDir = mc.WorkDir
		}
		if mc.UID != 0 {
			cUID = mc.UID
		}
		if mc.GID != 0 {
			cGID = mc.GID
		}
		if mc.InitCmd != "" {
			initCmd, err = shlex.Split(mc.InitCmd)
			if err != nil {
				logger.Error("failed to parse init_cmd", zap.String("init_cmd", mc.InitCmd), zap.Error(err))
				err = fmt.Errorf("failed to parse initCmd: %s %w", mc.InitCmd, err)
				return
			}
			logger.Info("initialize container with command", zap.String("init_cmd", mc.InitCmd))
		}
	}
	logger.Info("creating container builder",
		zap.String("host_name", hostName),
		zap.String("domain_name", domainName),
		zap.String("work_dir", workDir),
	)
	return
}

func tryClone3(
	c Config,
	envBuilder linuxcontainer.EnvironmentBuilder,
	cgb cgroup.Cgroup,
	cgroupType int,
	cgroupPool linuxcontainer.CgroupPool,
	workDir string,
	seccomp []syscall.SockFilter,
	logger *zap.Logger,
) pool.EnvBuilder {
	major, minor := kernelVersion()
	if cgb == nil || cgroupType != cgroup.TypeV2 || (major < 5 || (major == 5 && minor < 7)) {
		return nil
	}
	logger.Info("running kernel >= 5.7 with cgroup V2, trying faster clone3(CLONE_INTO_CGROUP)",
		zap.Int("major", major), zap.Int("minor", minor))

	b := linuxcontainer.NewEnvBuilder(linuxcontainer.Config{
		Builder:    envBuilder,
		CgroupPool: cgroupPool,
		WorkDir:    workDir,
		Cpuset:     c.Cpuset,
		CPURate:    c.EnableCPURate,
		Seccomp:    seccomp,
		CgroupFd:   true,
	})
	e, err := b.Build()
	if err != nil {
		logger.Info("environment build failed", zap.Error(err))
		return nil
	}
	defer e.Destroy()

	p, err := e.Execve(context.TODO(), envexec.ExecveParam{
		Args: []string{"/usr/bin/env"},
		Limit: envexec.Limit{
			Memory: 256 << 20,
			Proc:   1,
		},
	})
	if err != nil {
		logger.Info("environment run failed", zap.Error(err))
		return nil
	}
	<-p.Done()
	r := p.Result()
	if r.Status == runner.StatusRunnerError {
		logger.Info("environment result failed", zap.Stringer("result", r))
		return nil
	}
	return b
}

type credGen struct {
	cur atomic.Uint32
}

func newCredGen(start uint32) *credGen {
	rt := &credGen{}
	rt.cur.Store(start)
	return rt
}

func (c *credGen) Get() syscall.Credential {
	n := c.cur.Add(1)
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
			// misparse it.
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
