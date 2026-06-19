package settings

import "github.com/chamrong/ivideo-hls/internal/domain/video"

type AuthMethod string

const (
	AuthSSH   AuthMethod = "ssh"
	AuthHTTPS AuthMethod = "https"
)

// Settings is the unified config type. Merges what was previously split
// between appconfig.File (persistent) and pipeline.Config (runtime).
type Settings struct {
	// Identity / push
	RemoteURL        string
	PushURL          string // credential-bearing; never log
	AuthMethod       AuthMethod
	Token            string
	PublicURLPattern string

	// Encoding
	Quality     video.Quality
	Compression video.Compression
	PreCompress bool

	// Pipeline behaviour
	MaxParallel  int
	ParallelMode bool
	Push         bool
	Cleanup      bool
	KeepSource   bool

	// Discovery
	SourceDir string
	Recursive bool
	ScriptDir string

	// Recovery
	ResumeReuseCompressed bool
}

func Default(scriptDir string) Settings {
	return Settings{
		Quality:     video.QualityMedium,
		Compression: video.CompressionBalanced,
		Push:        true,
		Cleanup:     true,
		MaxParallel: 1,
		ScriptDir:   scriptDir,
		SourceDir:   scriptDir,
	}
}
