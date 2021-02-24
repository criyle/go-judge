package env

import (
	"fmt"
	"os"
	"path"

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

// Mounts defines mount points for the container.
type Mounts struct {
	Mount      []Mount `yaml:"mount"`
	WorkDir    string  `yaml:"workDir"`
	HostName   string  `yaml:"hostName"`
	DomainName string  `yaml:"domainName"`
	UID        int     `yaml:"uid"`
	GID        int     `yaml:"gid"`
	Proc       bool    `yaml:"proc"`
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
			return nil, fmt.Errorf("Invalid mount type")
		}
	}
	if m.Proc {
		b.WithProc()
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
		// work dir
		WithTmpfs("w", tmpFsConf).
		// tmp dir
		WithTmpfs("tmp", tmpFsConf)
}
