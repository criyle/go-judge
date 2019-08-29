package runner

import (
	"github.com/criyle/go-judge/types"
	"github.com/criyle/go-sandbox/deamon"
)

var env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}

func (r *runnerInstance) run(done <-chan struct{}, task *types.RunTask) *types.RunTaskResult {
	var result types.RunTaskResult

	waitDone := make(chan struct{})
	param := r.Language.Get(task.Language, task.Type)

	execParam := deamon.ExecveParam{
		Args: param.Args,
		Envv: env,
	}
	rc, err := r.Master1.Execve(waitDone, &execParam)
	if err != nil {
		result.Status = "JGF"
		return &result
	}
	rt := <-rc
	result.Status = rt.TraceStatus.Error()
	return &result
}
