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
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/criyle/go-sandbox/pkg/forkexec"
	"github.com/criyle/go-sandbox/pkg/mount"
	ddbus "github.com/godbus/dbus/v5"
	"github.com/google/shlex"
	"golang.org/x/sys/unix"
)

const (
	containerName      = "executor_server"
	defaultWorkDir     = "/w"
	containerCredStart = 10000
	containerCred      = 1000
)

// NewBuilder build a environment builder
func NewBuilder(c Config) (pool.EnvBuilder, map[string]any, error) {
	var (
		mountBuilder  *mount.Builder
		symbolicLinks []container.SymbolicLink
		maskPaths     []string
	)
	mc, err := readMountConfig(c.MountConf)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, err
		}
		c.Info("Mount.yaml(", c.MountConf, ") does not exists, use the default container mount")
		mountBuilder = getDefaultMount(c.TmpFsParam)
	} else {
		mountBuilder, err = parseMountConfig(mc)
		if err != nil {
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
	c.Info("Created container mount at:", mountBuilder)

	seccomp, err := readSeccompConf(c.SeccompConf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load seccomp config: %v", err)
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
				return nil, nil, fmt.Errorf("failed to parse initCmd: %s %w", mc.InitCmd, err)
			}
			c.Info("Initialize container with command: ", mc.InitCmd)
		}
	}
	c.Info("Creating container builder: hostName=", hostName, ", domainName=", domainName, ", workDir=", workDir)

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
	}
	cgb, err := newCgroup(c)
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
	return linuxcontainer.NewEnvBuilder(linuxcontainer.Config{
			Builder:    b,
			CgroupPool: cgroupPool,
			WorkDir:    workDir,
			Cpuset:     c.Cpuset,
			CPURate:    c.EnableCPURate,
			Seccomp:    seccomp,
		}), map[string]any{
			"cgroupType":   cgroupType,
			"mount":        m,
			"symbolicLink": symbolicLinks,
			"maskedPaths":  maskPaths,
			"hostName":     hostName,
			"domainName":   domainName,
			"workDir":      workDir,
			"uid":          cUID,
			"gid":          cGID,
		}, nil
}

func newCgroup(c Config) (cgroup.Cgroup, error) {
	prefix := c.CgroupPrefix
	t := cgroup.DetectedCgroupType
	ct, err := cgroup.GetAvailableController()
	if err != nil {
		c.Error("Failed to get available controllers", err)
		return nil, err
	}
	if t == cgroup.TypeV2 {
		// Check if running on a systemd enabled system
		c.Info("Running with cgroup v2, connecting systemd dbus to create cgroup")
		var conn *dbus.Conn
		if os.Getuid() == 0 {
			conn, err = dbus.NewSystemConnectionContext(context.TODO())
		} else {
			conn, err = dbus.NewUserConnectionContext(context.TODO())
		}
		if err != nil {
			c.Info("Connecting to systemd dbus failed:", err)
			c.Info("Assuming running in container, enable cgroup v2 nesting support and take control of the whole cgroupfs")
			prefix = ""
		} else {
			defer conn.Close()

			scopeName := c.CgroupPrefix + ".scope"
			c.Info("Connected to systemd bus, attempting to create transient unit: ", scopeName)

			properties := []dbus.Property{
				dbus.PropDescription("go judge - a high performance sandbox service base on container technologies"),
				dbus.PropWants(scopeName),
				dbus.PropPids(uint32(os.Getpid())),
				newSystemdProperty("Delegate", true),
			}
			ch := make(chan string, 1)
			if _, err := conn.StartTransientUnitContext(context.TODO(), scopeName, "replace", properties, ch); err != nil {
				return nil, fmt.Errorf("failed to start transient unit: %w", err)
			}
			s := <-ch
			if s != "done" {
				return nil, fmt.Errorf("starting transient unit returns error: %w", err)
			}
			scopeName, err := cgroup.GetCurrentCgroupPrefix()
			if err != nil {
				return nil, err
			}
			c.Info("Current cgroup is ", scopeName)
			prefix = scopeName
			ct, err = cgroup.GetAvailableControllerWithPrefix(prefix)
			if err != nil {
				return nil, err
			}
		}
	}
	cgb, err := cgroup.New(prefix, ct)
	if err != nil {
		if os.Getuid() == 0 {
			c.Error("Failed to create cgroup ", prefix, " ", err)
			return nil, err
		}
		c.Warn("Not running in root and have no permission on cgroup, falling back to rlimit / rusage mode")
		return nil, nil
	}
	// Create api and migrate current process into it
	c.Info("Creating nesting api cgroup ", cgb)
	if _, err = cgb.Nest("api"); err != nil {
		// Only allow to fall back to rlimit mode when not running with root
		if os.Getuid() != 0 {
			c.Warn("Creating api cgroup with error: ", err)
			c.Warn("As running in non-root mode, falling back back to rlimit / rusage mode")
			cgb.Destroy()
			return nil, nil
		}
	}

	c.Info("Creating containers cgroup")
	cg, err := cgb.New("containers")
	if err != nil {
		c.Warn("Creating containers cgroup with error: ", err)
		c.Warn("Falling back to rlimit / rusage mode")
		cgb = nil
	}
	return cg, nil
}

func newSystemdProperty(name string, units any) dbus.Property {
	return dbus.Property{
		Name:  name,
		Value: ddbus.MakeVariant(units),
	}
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
