package job_test

import (
	"testing"

	"github.com/chamrong/ivideo-hls/internal/domain/job"
)

func TestFuncEmitter_Calls(t *testing.T) {
	var got []job.Event
	e := job.FuncEmitter(func(ev job.Event) { got = append(got, ev) })
	e.Emit(job.Event{Job: "v1", Message: "hello"})
	if len(got) != 1 || got[0].Message != "hello" {
		t.Fatalf("got %v", got)
	}
}

func TestNilEmitter_DoesNotPanic(t *testing.T) {
	var e job.Emitter // nil interface
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil emitter panicked: %v", r)
		}
	}()
	job.Emit(e, job.LevelInfo, "j", job.StageCompress, "msg")
}
