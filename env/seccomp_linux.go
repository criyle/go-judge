package env

import (
	"fmt"
	"os"
	"syscall"

	seccompprofile "github.com/criyle/go-judge/seccomp"
	"github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"github.com/elastic/go-ucfg/yaml"
	"golang.org/x/net/bpf"
)

type architectureSeccompConfig struct {
	DefaultAction seccomp.Action                       `config:"default_action" yaml:"default_action"`
	Base          architectureSeccompPolicy            `config:"base" yaml:"base"`
	Architectures map[string]architectureSeccompPolicy `config:"architectures" yaml:"architectures"`
}

type architectureSeccompPolicy struct {
	Syscalls []seccomp.SyscallGroup `config:"syscalls" yaml:"syscalls"`
}

func readSeccompConf(name string) ([]syscall.SockFilter, error) {
	var input []byte
	if name == "" {
		input = seccompprofile.DefaultProfile
	} else {
		var err error
		input, err = os.ReadFile(name)
		if err != nil {
			return nil, err
		}
	}
	conf, err := yaml.NewConfig(input)
	if err != nil {
		return nil, err
	}

	var config architectureSeccompConfig
	if err := conf.Unpack(&config); err != nil {
		return nil, err
	}
	info, err := arch.GetInfo("")
	if err != nil {
		return nil, fmt.Errorf("seccomp architecture: %w", err)
	}
	selected, ok := config.Architectures[info.Name]
	if !ok {
		return nil, fmt.Errorf("seccomp profile has no configuration for architecture %q", info.Name)
	}
	policy := seccomp.Policy{
		DefaultAction: config.DefaultAction,
		Syscalls:      append(append([]seccomp.SyscallGroup{}, config.Base.Syscalls...), selected.Syscalls...),
	}
	inst, err := policy.Assemble()
	if err != nil {
		return nil, err
	}
	rawInst, err := bpf.Assemble(inst)
	if err != nil {
		return nil, err
	}
	return toSockFilter(rawInst), nil
}

func toSockFilter(raw []bpf.RawInstruction) []syscall.SockFilter {
	filter := make([]syscall.SockFilter, 0, len(raw))
	for _, instruction := range raw {
		filter = append(filter, syscall.SockFilter{
			Code: instruction.Op,
			Jt:   instruction.Jt,
			Jf:   instruction.Jf,
			K:    instruction.K,
		})
	}
	return filter
}
