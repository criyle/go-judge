package env

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/criyle/go-judge/env/linuxcontainer"
	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/forkexec"
	"github.com/criyle/go-sandbox/pkg/mount"
	"github.com/criyle/go-sandbox/runner"
	ddbus "github.com/godbus/dbus/v5"
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
	var (
		mountBuilder  *mount.Builder
		symbolicLinks []container.SymbolicLink
		maskPaths     []string
		unshareCgroup bool = true
	)
	mc, err := readMountConfig(c.MountConf)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Error("failed to read mount config", zap.String("path", c.MountConf), zap.Error(err))
			return nil, nil, err
		}
		logger.Info("mount.yaml does not exist, using default container mount", zap.String("path", c.MountConf))
		mountBuilder = getDefaultMount(c.TmpFsParam)
	} else {
		mountBuilder, err = parseMountConfig(mc)
		if err != nil {
			logger.Error("failed to parse mount config", zap.Error(err))
			return nil, nil, err
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
	logger.Info("created container mount", zap.Any("mountBuilder", mountBuilder))

	seccomp, err := readSeccompConf(c.SeccompConf)
	if err != nil {
		logger.Error("failed to load seccomp config", zap.String("path", c.SeccompConf), zap.Error(err))
		return nil, nil, fmt.Errorf("failed to load seccomp config: %w", err)
	}
	if seccomp != nil {
		logger.Info("loaded seccomp filter", zap.String("path", c.SeccompConf))
	}

	unshareFlags := uintptr(forkexec.UnshareFlags)
	if c.NetShare {
		unshareFlags ^= syscall.CLONE_NEWNET
	}
	major, minor := kernelVersion()
	unshareFlags ^= unix.CLONE_NEWCGROUP
	if major < 4 || (major == 4 && minor < 6) {
		unshareCgroup = false
		logger.Info("kernel version < 4.6, don't unshare cgroup", zap.Int("major", major), zap.Int("minor", minor))
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
	var initCmd []string
	if mc != nil {
		hostName = mc.HostName
		domainName = mc.DomainName
		workDir = mc.WorkDir
		cUID = mc.UID
		cGID = mc.GID
		if mc.InitCmd != "" {
			initCmd, err = shlex.Split(mc.InitCmd)
			if err != nil {
				logger.Error("failed to parse init_cmd", zap.String("init_cmd", mc.InitCmd), zap.Error(err))
				return nil, nil, fmt.Errorf("failed to parse initCmd: %s %w", mc.InitCmd, err)
			}
			logger.Info("initialize container with command", zap.String("init_cmd", mc.InitCmd))
		}
	}
	logger.Info("creating container builder",
		zap.String("host_name", hostName),
		zap.String("domain_name", domainName),
		zap.String("work_dir", workDir),
	)

	b := &container.Builder{
		TmpRoot:       "go-judge",
		Mounts:        m,
		SymbolicLinks: symbolicLinks,
		MaskPaths:     maskPaths,
		CredGenerator: credGen,
		Stderr:        os.Stderr,
		CloneFlags:    unshareFlags,
		ExecFile:      c.ContainerInitPath,
		HostName:      hostName,
		DomainName:    domainName,
		InitCommand:   initCmd,
		WorkDir:       workDir,
		ContainerUID:  cUID,
		ContainerGID:  cGID,

		UnshareCgroupBeforeExec: unshareCgroup,
	}
	cgb, ct, err := newCgroup(c, logger)
	if err != nil {
		return nil, nil, err
	}

	var cgroupPool linuxcontainer.CgroupPool
	if cgb != nil {
		cgroupPool = linuxcontainer.NewFakeCgroupPool(cgb, c.CPUCfsPeriod)
	}
	cgroupType := int(cgroup.DetectedCgroupType)
	if cgb == nil {
		cgroupType = 0
	}
	cgroupControllers := []string{}
	if ct != nil {
		cgroupControllers = ct.Names()
	}
	conf := map[string]any{
		"cgroupType":   cgroupType,
		"mount":        m,
		"symbolicLink": symbolicLinks,
		"maskedPaths":  maskPaths,
		"hostName":     hostName,
		"domainName":   domainName,
		"workDir":      workDir,
		"uid":          cUID,
		"gid":          cGID,

		"cgroupControllers": cgroupControllers,
	}
	if cgb != nil && cgroupType == cgroup.TypeV2 && (major > 5 || major == 5 && minor >= 7) {
		logger.Info("running kernel >= 5.7 with cgroup V2, trying faster clone3(CLONE_INTO_CGROUP)",
			zap.Int("major", major), zap.Int("minor", minor))
		if b := func() pool.EnvBuilder {
			b := linuxcontainer.NewEnvBuilder(linuxcontainer.Config{
				Builder:    b,
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
		}(); b != nil {
			conf["clone3"] = true
			return b, conf, nil
		}
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

func newCgroup(c Config, logger *zap.Logger) (cgroup.Cgroup, *cgroup.Controllers, error) {
	prefix := c.CgroupPrefix
	t := cgroup.DetectedCgroupType
	ct, err := cgroup.GetAvailableController()
	if err != nil {
		logger.Error("failed to get available controllers", zap.Error(err))
		return nil, nil, err
	}
	if t == cgroup.TypeV2 {
		logger.Info("running with cgroup v2, connecting systemd dbus to create cgroup")
		var conn *dbus.Conn
		if os.Getuid() == 0 {
			conn, err = dbus.NewSystemConnectionContext(context.TODO())
		} else {
			conn, err = dbus.NewUserConnectionContext(context.TODO())
		}
		if err != nil {
			logger.Info("connecting to systemd dbus failed, assuming running in container, enable cgroup v2 nesting support and take control of the whole cgroupfs", zap.Error(err))
			prefix = ""
		} else {
			defer conn.Close()

			scopeName := c.CgroupPrefix + ".scope"
			logger.Info("connected to systemd bus, attempting to create transient unit", zap.String("scopeName", scopeName))

			properties := []dbus.Property{
				dbus.PropDescription("go judge - a high performance sandbox service base on container technologies"),
				dbus.PropWants(scopeName),
				dbus.PropPids(uint32(os.Getpid())),
				newSystemdProperty("Delegate", true),
			}
			ch := make(chan string, 1)
			if _, err := conn.StartTransientUnitContext(context.TODO(), scopeName, "replace", properties, ch); err != nil {
				logger.Error("failed to start transient unit", zap.Error(err))
				return nil, nil, fmt.Errorf("failed to start transient unit: %w", err)
			}
			s := <-ch
			if s != "done" {
				logger.Error("starting transient unit returns error", zap.String("status", s), zap.Error(err))
				return nil, nil, fmt.Errorf("starting transient unit returns error: %w", err)
			}
			scopeName, err := cgroup.GetCurrentCgroupPrefix()
			if err != nil {
				logger.Error("failed to get current cgroup prefix", zap.Error(err))
				return nil, nil, err
			}
			logger.Info("current cgroup", zap.String("scope_name", scopeName))
			prefix = scopeName
			ct, err = cgroup.GetAvailableControllerWithPrefix(prefix)
			if err != nil {
				logger.Error("failed to get available controller with prefix", zap.Error(err))
				return nil, nil, err
			}
		}
	}
	cgb, err := cgroup.New(prefix, ct)
	if err != nil {
		if os.Getuid() == 0 {
			logger.Error("failed to create cgroup", zap.String("prefix", prefix), zap.Error(err))
			return nil, nil, err
		}
		logger.Warn("not running in root and have no permission on cgroup, falling back to rlimit / rusage mode", zap.Error(err))
		return nil, nil, nil
	}
	logger.Info("creating nesting api cgroup", zap.Any("cgroup", cgb))
	if _, err = cgb.Nest("api"); err != nil {
		if os.Getuid() != 0 {
			logger.Warn("creating api cgroup with error, falling back to rlimit / rusage mode", zap.Error(err))
			cgb.Destroy()
			return nil, nil, nil
		}
	}

	logger.Info("creating containers cgroup")
	cg, err := cgb.New("containers")
	if err != nil {
		logger.Warn("creating containers cgroup with error, falling back to rlimit / rusage mode", zap.Error(err))
		cgb = nil
	}
	if !ct.Memory {
		logger.Warn("memory cgroup is not enabled, falling back to rlimit / rusage mode")
	}
	if !ct.Pids {
		logger.Warn("pid cgroup is not enabled, proc limit does not have effect")
	}
	return cg, ct, nil
}

func newSystemdProperty(name string, units any) dbus.Property {
	return dbus.Property{
		Name:  name,
		Value: ddbus.MakeVariant(units),
	}
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
