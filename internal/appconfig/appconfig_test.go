package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoad_Roundtrip(t *testing.T) {
	withXDGHome(t)

	original := File{
		RemoteURL:              "git@example.com:org/repo.git",
		AuthMethod:             AuthSSH,
		Token:                  "ghp_xxxxxxx",
		DefaultQuality:         "high",
		DefaultCompression:     "best",
		DefaultPreCompress:     true,
		DefaultKeepSource:      false,
		DefaultSourceDir:       "/Users/me/Videos",
		DefaultPushDisabled:    false,
		DefaultCleanupDisabled: true,
		DefaultParallel:        3,
		DefaultRecursive:       true,
		PublicURLPattern:       "https://raw.example.com/{branch}/x/{filename}",
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != original {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, original)
	}
}

func TestLoad_MissingFileReturnsZero(t *testing.T) {
	withXDGHome(t)
	got, err := Load()
	if err != nil {
		t.Fatalf("Load with no file should not error: %v", err)
	}
	if got != (File{}) {
		t.Errorf("expected zero-value File, got %+v", got)
	}
}

func TestSave_Mode0600(t *testing.T) {
	withXDGHome(t)
	if err := Save(File{RemoteURL: "git@x:y/z.git"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, _ := Path()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("config mode = %o, want 0600 (token lives here)", mode)
	}
}

func TestEffectiveRemoteURL(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		token  string
		method AuthMethod
		want   string
	}{
		{
			name: "ssh passthrough", url: "git@host:org/repo.git",
			token: "ignored", method: AuthSSH,
			want: "git@host:org/repo.git",
		},
		{
			name: "https with token injects userinfo",
			url:  "https://github.com/org/repo.git", token: "ghp_abc",
			method: AuthHTTPS,
			want:   "https://ghp_abc@github.com/org/repo.git",
		},
		{
			name: "https without token unchanged",
			url:  "https://github.com/org/repo.git", token: "",
			method: AuthHTTPS,
			want:   "https://github.com/org/repo.git",
		},
		{
			name: "already-credentialed URL not double-injected",
			url:  "https://user@github.com/org/repo.git", token: "ghp_abc",
			method: AuthHTTPS,
			want:   "https://user@github.com/org/repo.git",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EffectiveRemoteURL(c.url, c.token, c.method)
			if got != c.want {
				t.Errorf("\n got: %q\nwant: %q", got, c.want)
			}
		})
	}
}

func TestMaskToken(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"abc":           "•••",
		"12345678":      "••••••••",
		"abcdefghijkl":  "••••••••ijkl",
	}
	for in, want := range cases {
		if got := MaskToken(in); got != want {
			t.Errorf("MaskToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateRemoteURL(t *testing.T) {
	ok := []string{
		"git@github.com:org/repo.git",
		"ssh://git@gitlab.com/org/repo.git",
		"https://github.com/org/repo.git",
	}
	for _, u := range ok {
		if err := ValidateRemoteURL(u); err != nil {
			t.Errorf("ValidateRemoteURL(%q) unexpected error: %v", u, err)
		}
	}
	bad := []string{"", "file:///path", "ftp://example.com/repo.git", "github.com/x/y"}
	for _, u := range bad {
		if err := ValidateRemoteURL(u); err == nil {
			t.Errorf("ValidateRemoteURL(%q) expected error, got nil", u)
		}
	}
}

func TestInferAuthMethod(t *testing.T) {
	cases := []struct {
		url     string
		current AuthMethod
		want    AuthMethod
	}{
		{"https://github.com/x/y.git", AuthSSH, AuthHTTPS},
		{"git@github.com:x/y.git", AuthHTTPS, AuthSSH},
		{"ssh://git@gitlab.com/x.git", AuthHTTPS, AuthSSH},
		{"", AuthSSH, AuthSSH}, // unknown URL keeps current
	}
	for _, c := range cases {
		got := InferAuthMethod(c.url, c.current)
		if got != c.want {
			t.Errorf("InferAuthMethod(%q, %q) = %q, want %q", c.url, c.current, got, c.want)
		}
	}
}

func TestParseLine(t *testing.T) {
	cases := []struct {
		in    string
		key   string
		value string
		ok    bool
	}{
		{`remote_url = "git@host:x.git"`, "remote_url", "git@host:x.git", true},
		{`default_parallel = 3`, "default_parallel", "3", true},
		{"# comment", "", "", false},
		{"", "", "", false},
		{"not-an-assignment", "", "", false},
		{`  key   =   value  `, "key", "value", true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			k, v, ok := parseLine(c.in)
			if k != c.key || v != c.value || ok != c.ok {
				t.Errorf("parseLine(%q) = (%q, %q, %v), want (%q, %q, %v)",
					c.in, k, v, ok, c.key, c.value, c.ok)
			}
		})
	}
}

// withXDGHome redirects the config-file location to a per-test temp dir so
// tests neither read nor write the real user config.
func withXDGHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// sanity check that Path honours the env var
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !strings.HasPrefix(p, dir) {
		t.Fatalf("Path %q did not honour XDG_CONFIG_HOME %q", p, dir)
	}
	_ = filepath.Dir(p)
}
