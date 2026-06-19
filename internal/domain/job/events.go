package job

import "time"

type EventLevel int

const (
	LevelInfo    EventLevel = iota
	LevelSuccess
	LevelWarn
	LevelError
	LevelDim
)

type Event struct {
	Time    time.Time
	Job     string
	Stage   Stage
	Level   EventLevel
	Message string
	Percent float64
	Speed   float64
	Bitrate string
}

type Emitter interface {
	Emit(Event)
}

type FuncEmitter func(Event)

func (f FuncEmitter) Emit(ev Event) { f(ev) }

func Emit(e Emitter, level EventLevel, jobName string, stage Stage, msg string) {
	if e == nil {
		return
	}
	e.Emit(Event{Time: time.Now(), Job: jobName, Stage: stage, Level: level, Message: msg})
}

func EmitProgress(e Emitter, jobName string, stage Stage, pct float64, msg string, speed float64, bitrate string) {
	if e == nil {
		return
	}
	e.Emit(Event{Time: time.Now(), Job: jobName, Stage: stage, Level: LevelDim,
		Percent: pct, Message: msg, Speed: speed, Bitrate: bitrate})
}
