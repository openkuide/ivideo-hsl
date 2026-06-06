package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type runOpts struct {
	cwd     string
	job     string
	stage   Stage
	emitter Emitter
	silent  bool
	// onStdout receives each stdout line before logging. Return true to suppress
	// the default dim log for that line (useful for structured progress streams).
	onStdout func(line string) bool
}

func run(ctx context.Context, opts runOpts, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if opts.cwd != "" {
		cmd.Dir = opts.cwd
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var lastErrLine string
	go streamLines(stdout, func(line string) {
		if opts.onStdout != nil && opts.onStdout(line) {
			return
		}
		if opts.silent || opts.emitter == nil {
			return
		}
		dim(opts.emitter, opts.job, opts.stage, line)
	})
	go streamLines(stderr, func(line string) {
		lastErrLine = line
		if opts.silent || opts.emitter == nil {
			return
		}
		warn(opts.emitter, opts.job, opts.stage, line)
	})

	if err := cmd.Wait(); err != nil {
		cmdline := redactedCommandLine(name, args)
		if lastErrLine != "" {
			return fmt.Errorf("%s: %w: %s", cmdline, err, redactCredentials(lastErrLine))
		}
		return fmt.Errorf("%s: %w", cmdline, err)
	}
	return nil
}

func streamLines(r io.Reader, fn func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if line == "" {
			continue
		}
		fn(line)
	}
}

func runQuiet(ctx context.Context, cwd, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s",
			redactedCommandLine(name, args),
			err,
			redactCredentials(strings.TrimSpace(string(out))))
	}
	return nil
}

func runCapture(ctx context.Context, cwd, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
