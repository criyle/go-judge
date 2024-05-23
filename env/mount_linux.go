package env

import (
	"fmt"
	"os"
	"path"

	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/mount"
	"gopkg.in/yaml.v2"
)

// Mount defines single mount point configuration.
// type could be bind / tmpfs
type Mount struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	Readonly bool   `yaml:"readonly"`
	Data     string `yaml:"data"`
}

// Link defines symlinks to be created after mounts
type Link struct {
	LinkPath string `yaml:"linkPath"`
	Target   string `yaml:"target"`
}

// Mounts defines mount points for the container.
type Mounts struct {
	Mount      []Mount  `yaml:"mount"`
	SymLinks   []Link   `yaml:"symLink"`
	MaskPaths  []string `yaml:"maskPath"`
	InitCmd    string   `yaml:"initCmd"`
	WorkDir    string   `yaml:"workDir"`
	HostName   string   `yaml:"hostName"`
	DomainName string   `yaml:"domainName"`
	UID        int      `yaml:"uid"`
	GID        int      `yaml:"gid"`
	Proc       bool     `yaml:"proc"`
	ProcRW     bool     `yaml:"procrw"`
}

func readMountConfig(p string) (*Mounts, error) {
	var m Mounts
	d, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(d, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func parseMountConfig(m *Mounts) (*mount.Builder, error) {
	b := mount.NewBuilder()
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for _, mt := range m.Mount {
		target := mt.Target
		if path.IsAbs(target) {
			target = path.Clean(target[1:])
		}
		source := mt.Source
		if !path.IsAbs(source) {
			source = path.Join(wd, source)
		}
		switch mt.Type {
		case "bind":
			b.WithBind(source, target, mt.Readonly)
		case "tmpfs":
			b.WithTmpfs(target, mt.Data)
		default:
			return nil, fmt.Errorf("invalid_mount_type: %v", mt.Type)
		}
	}
	if m.Proc {
		b.WithProcRW(m.ProcRW)
	}
	return b, nil
}

func getDefaultMount(tmpFsConf string) *mount.Builder {
	return mount.NewBuilder().
		// basic exec and lib
		WithBind("/bin", "bin", true).
		WithBind("/lib", "lib", true).
		WithBind("/lib64", "lib64", true).
		WithBind("/usr", "usr", true).
		WithBind("/etc/ld.so.cache", "etc/ld.so.cache", true).
		// java wants /proc/self/exe as it need relative path for lib
		// however, /proc gives interface like /proc/1/fd/3 ..
		// it is fine since open that file will be a EPERM
		// changing the fs uid and gid would be a good idea
		WithProc().
		// some compiler have multiple version
		WithBind("/etc/alternatives", "etc/alternatives", true).
		// fpc wants /etc/fpc.cfg
		WithBind("/etc/fpc.cfg", "etc/fpc.cfg", true).
		// mono wants /etc/mono
		WithBind("/etc/mono", "etc/mono", true).
		// go wants /dev/null
		WithBind("/dev/null", "dev/null", false).
		// ghc wants /var/lib/ghc
		WithBind("/var/lib/ghc", "var/lib/ghc", true).
		// javaScript wants /dev/urandom
		WithBind("/dev/urandom", "dev/urandom", false).
		// additional devices
		WithBind("/dev/random", "dev/random", false).
		WithBind("/dev/zero", "dev/zero", false).
		WithBind("/dev/full", "dev/full", false).
		// work dir
		WithTmpfs("w", tmpFsConf).
		// tmp dir
		WithTmpfs("tmp", tmpFsConf)
}

var defaultSymLinks = []container.SymbolicLink{
	{LinkPath: "/dev/fd", Target: "/proc/self/fd"},
	{LinkPath: "/dev/stdin", Target: "/proc/self/fd/0"},
	{LinkPath: "/dev/stdout", Target: "/proc/self/fd/1"},
	{LinkPath: "/dev/stderr", Target: "/proc/self/fd/2"},
}

var defaultMaskPaths = []string{
	"/sys/firmware",
	"/sys/devices/virtual/powercap",
	"/proc/acpi",
	"/proc/asound",
	"/proc/kcore",
	"/proc/keys",
	"/proc/latency_stats",
	"/proc/timer_list",
	"/proc/timer_stats",
	"/proc/sched_debug",
	"/proc/scsi",
	"/usr/lib/wsl/drivers",
	"/usr/lib/wsl/lib",
}
