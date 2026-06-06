package pipeline

import "time"

type EventLevel int

const (
	LevelInfo EventLevel = iota
	LevelSuccess
	LevelWarn
	LevelError
	LevelDim
)

type Stage string

const (
	StageQueued    Stage = "queued"
	StageWorkspace Stage = "workspace"
	StageCompress  Stage = "compress"
	StageConvert   Stage = "convert"
	StageRename    Stage = "rename"
	StageGitPush   Stage = "push"
	StageDone      Stage = "done"
	StageFailed    Stage = "failed"
)

// BaseJob identifies events that belong to the one-time base-hero preparation
// rather than to a specific video. It is the only reserved job name.
const BaseJob = "base"

type Event struct {
	Time    time.Time
	Job     string
	Stage   Stage
	Level   EventLevel
	Message string
	Percent float64
	// Speed is the ffmpeg encoding speed (e.g. 2.1 means 2.1x realtime).
	// Zero when unknown — present only on progress-tick events.
	Speed float64
	// Bitrate is ffmpeg's current output bitrate string (e.g. "2800.0kbits/s").
	// Empty when unknown — present only on progress-tick events.
	Bitrate string
}

type Emitter interface {
	Emit(ev Event)
}

type FuncEmitter func(Event)

func (f FuncEmitter) Emit(ev Event) { f(ev) }

// emit is the single nil-safe entry point all the level helpers funnel
// through. A nil emitter is legitimate (tests, plain-exec paths) and must
// never panic at this layer.
func emit(e Emitter, level EventLevel, job string, stage Stage, msg string) {
	if e == nil {
		return
	}
	e.Emit(Event{Time: time.Now(), Job: job, Stage: stage, Level: level, Message: msg})
}

func info(e Emitter, job string, stage Stage, msg string) {
	emit(e, LevelInfo, job, stage, msg)
}

func success(e Emitter, job string, stage Stage, msg string) {
	emit(e, LevelSuccess, job, stage, msg)
}

func warn(e Emitter, job string, stage Stage, msg string) {
	emit(e, LevelWarn, job, stage, msg)
}

func errorf(e Emitter, job string, stage Stage, msg string) {
	emit(e, LevelError, job, stage, msg)
}

func dim(e Emitter, job string, stage Stage, msg string) {
	emit(e, LevelDim, job, stage, msg)
}

func emitProgress(e Emitter, job string, stage Stage, pct float64, msg string, speed float64, bitrate string) {
	if e == nil {
		return
	}
	e.Emit(Event{
		Time:    time.Now(),
		Job:     job,
		Stage:   stage,
		Level:   LevelDim,
		Percent: pct,
		Message: msg,
		Speed:   speed,
		Bitrate: bitrate,
	})
}
