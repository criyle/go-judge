package wsexecutor

import (
	"errors"
	"testing"

	"github.com/criyle/go-judge/cmd/go-judge/model"
)

func TestValidateStreamRequestLimitsRejectsLargeCommandIndex(t *testing.T) {
	req := &model.Request{
		Cmd: make([]model.Cmd, maxPackedStreamField+2),
	}
	req.Cmd[maxPackedStreamField+1].Files = []*model.CmdFile{{StreamOut: true}}

	err := validateStreamRequestLimits(req)
	if !errors.Is(err, errStreamFieldTooLarge) {
		t.Fatalf("expected stream field limit error, got %v", err)
	}
}

func TestValidateStreamRequestLimitsRejectsLargeFD(t *testing.T) {
	req := &model.Request{
		Cmd: []model.Cmd{{
			Files: make([]*model.CmdFile, maxPackedStreamField+2),
		}},
	}
	req.Cmd[0].Files[maxPackedStreamField+1] = &model.CmdFile{StreamIn: true}

	err := validateStreamRequestLimits(req)
	if !errors.Is(err, errStreamFieldTooLarge) {
		t.Fatalf("expected stream field limit error, got %v", err)
	}
}

func TestValidateStreamRequestLimitsAllowsBoundaryValues(t *testing.T) {
	req := &model.Request{
		Cmd: make([]model.Cmd, maxPackedStreamField+1),
	}
	req.Cmd[maxPackedStreamField].Files = make([]*model.CmdFile, maxPackedStreamField+1)
	req.Cmd[maxPackedStreamField].Files[maxPackedStreamField] = &model.CmdFile{StreamOut: true}

	if err := validateStreamRequestLimits(req); err != nil {
		t.Fatalf("expected boundary values to pass, got %v", err)
	}
}
