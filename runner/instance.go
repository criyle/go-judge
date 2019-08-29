package runner

import (
	"fmt"

	"github.com/criyle/go-sandbox/deamon"
)

type runnerInstance struct {
	*Runner
	Master1 *deamon.Master
	Master2 *deamon.Master
}

func (r *Runner) newInstance() (*runnerInstance, error) {
	m1, err := deamon.New(r.Root)
	if err != nil {
		return nil, fmt.Errorf("instance: failed to create deamon %v", err)
	}
	m2, err := deamon.New(r.Root)
	if err != nil {
		return nil, fmt.Errorf("instance: failed to create deamon2 %v", err)
	}
	return &runnerInstance{
		Runner:  r,
		Master1: m1,
		Master2: m2,
	}, nil
}
