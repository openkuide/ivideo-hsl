package pipeline

import (
	"regexp"
	"strings"
)

// Credential patterns that must never appear in logs, error messages, or
// committed artifacts. Keep this list narrow — false positives are better
// than false negatives here.
var (
	// URL userinfo: https://TOKEN@host/path or https://user:TOKEN@host/path.
	// The token sits in the authority section before the @.
	urlCredentialPattern = regexp.MustCompile(`(https?://)([^/@\s]+)@`)

	// GitHub fine-grained PAT: `github_pat_` followed by base62-ish.
	// Classic token: `ghp_` + 36 chars. OAuth: `gho_`. App: `ghs_`, `ghu_`, `ghr_`.
	// GitLab PAT: `glpat-` followed by the token body.
	githubTokenPattern = regexp.MustCompile(
		`(?:github_pat_[A-Za-z0-9_]+|gh[opsur]_[A-Za-z0-9]+|glpat-[A-Za-z0-9_.-]+)`)
)

// redactCredentials returns s with recognized secrets replaced by a fixed
// placeholder. Used at the log/error boundary — never trust the raw command
// line to be safe for humans to read.
func redactCredentials(s string) string {
	s = urlCredentialPattern.ReplaceAllString(s, "${1}***@")
	s = githubTokenPattern.ReplaceAllString(s, "***")
	return s
}

// redactArgs returns a copy of args with credentials redacted in each
// element. Safe to pass to strings.Join for human-readable logging.
func redactArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = redactCredentials(a)
	}
	return out
}

// redactedCommandLine joins name + args with credentials scrubbed. Use this
// anywhere a command line is about to land in an error message or log.
func redactedCommandLine(name string, args []string) string {
	return name + " " + strings.Join(redactArgs(args), " ")
}
