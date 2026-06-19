package job

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

const BaseJob = "base"

type Result struct {
	VideoPath string
	Success   bool
	Err       error
}

type IncompleteWorkspace struct {
	Name           string
	Workspace      string
	SourcePath     string
	CompressedPath string
	Stage          Stage
	Hint           string
	SourceExists   bool
}

type RetryWorkspace struct {
	Name      string
	Workspace string
	Branch    string
	Size      int64
}
