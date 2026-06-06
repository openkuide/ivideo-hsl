package pipeline

import (
	"strings"
	"testing"
)

func TestRedactCredentials(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "url with github fine-grained PAT",
			in:   "https://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@github.com/username/repo.git",
			want: "https://***@github.com/username/repo.git",
		},
		{
			name: "url with classic ghp_ token",
			in:   "git push -u -f https://ghp_abcdef1234567890abcdef1234567890ABCD@github.com/x/y.git",
			want: "git push -u -f https://***@github.com/x/y.git",
		},
		{
			name: "url with gitlab glpat",
			in:   "https://oauth2:glpat-Bpko0u8fHakFdRaaUvMylW86MQp1@gitlab.com/username/repo.git",
			want: "https://***@gitlab.com/username/repo.git",
		},
		{
			name: "bare github PAT in error text",
			in:   "fatal: bad credentials github_pat_11B3YKAYA0dM43LmVcVCjF",
			want: "fatal: bad credentials ***",
		},
		{
			name: "bare ghs_ token",
			in:   "token=ghs_1234567890abcdef",
			want: "token=***",
		},
		{
			name: "ssh url passes through",
			in:   "git@github.com:username/repo.git",
			want: "git@github.com:username/repo.git",
		},
		{
			name: "plain https without userinfo passes through",
			in:   "fatal: unable to access 'https://github.com/username/repo.git/'",
			want: "fatal: unable to access 'https://github.com/username/repo.git/'",
		},
		{
			name: "no-op when empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactCredentials(tc.in)
			if got != tc.want {
				t.Errorf("redactCredentials(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactCredentials_NeverLeaksKnownPatterns(t *testing.T) {
	// If any of these fragments survive redaction, we have a real leak.
	dangerous := []string{
		"github_pat_11B3YKAYA0dM43LmVcVCjF",
		"ghp_abcdef1234567890abcdef1234",
		"gho_123456abcdef",
		"ghs_987654fedcba",
		"glpat-Bpko0u8fHakFdRaaUvMylW86MQp1",
	}
	for _, d := range dangerous {
		t.Run(d, func(t *testing.T) {
			out := redactCredentials("prefix " + d + " suffix")
			if strings.Contains(out, d) {
				t.Fatalf("credential survived: %q", out)
			}
		})
	}
}

func TestRedactedCommandLine(t *testing.T) {
	got := redactedCommandLine("git", []string{"push", "-u", "-f",
		"https://github_pat_ABCDEF@github.com/x/y.git", "main"})
	want := "git push -u -f https://***@github.com/x/y.git main"
	if got != want {
		t.Errorf("\n got: %q\nwant: %q", got, want)
	}
}
