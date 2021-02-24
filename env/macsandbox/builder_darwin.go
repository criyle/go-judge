package macsandbox

import (
	"os"

	"github.com/criyle/go-judge/env/pool"
)

var _ pool.EnvBuilder = &Builder{}

// Builder to create sandbox environment
type Builder struct {
	wd                         string
	readablePath, writablePath []string
	network                    bool
}

// NewBuilder creates Builder to create sandbox environment
func NewBuilder(wd string, readablePath, writablePath []string, network bool) pool.EnvBuilder {
	return &Builder{
		wd:           wd,
		readablePath: readablePath,
		writablePath: writablePath,
		network:      network,
	}
}

// Build create a sandbox environment
func (b *Builder) Build() (pool.Environment, error) {
	wd, err := os.MkdirTemp(b.wd, "es")
	if err != nil {
		return nil, err
	}

	var rp, wp []string
	rp = append(rp, b.readablePath...)
	rp = append(rp, wd)

	wp = append(wp, b.writablePath...)
	wp = append(wp, wd)

	p := &Profile{
		WritableDir: wp,
		ReadableDir: rp,
		Network:     b.network,
	}
	profile, err := p.Build()
	if err != nil {
		return nil, err
	}

	wdf, err := os.Open(wd)
	if err != nil {
		return nil, err
	}

	return &environment{
		profile: profile,
		wdPath:  wd,
		wd:      wdf,
	}, nil
}
