//go:build seccomp

package env

import (
	"os"
	"syscall"

	"github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-ucfg/yaml"
	"golang.org/x/net/bpf"
)

func readSeccompConf(name string) ([]syscall.SockFilter, error) {
	conf, err := yaml.NewConfigWithFile(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var policy seccomp.Policy
	if err := conf.Unpack(&policy); err != nil {
		return nil, err
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
