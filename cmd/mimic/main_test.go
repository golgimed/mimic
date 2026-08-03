package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildMimic compiles the mimic binary once for the boot-smoke tests below.
func buildMimic(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mimic")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// runAndCapture starts the built binary in workdir and returns combined
// stdout/stderr collected until either "listening on" appears (boot
// succeeded) or the process exits (boot failed), whichever comes first.
func runAndCapture(t *testing.T, bin, workdir string, args ...string) (output string, exited bool, exitErr error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "DB_PATH=:memory:", "PORT=0")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	lines := make(chan string, 64)
	readInto := func(r interface{ Read([]byte) (int, error) }) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}
	go readInto(stdout)
	go readInto(stderr)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var collected strings.Builder
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line := <-lines:
			collected.WriteString(line)
			collected.WriteString("\n")
			if strings.Contains(line, "listening on") {
				_ = cmd.Process.Kill()
				<-done
				return collected.String(), false, nil
			}
		case err := <-done:
			return collected.String(), true, err
		case <-deadline:
			_ = cmd.Process.Kill()
			<-done
			return collected.String(), false, nil
		}
	}
}

func TestBootSucceedsWithEmptySpecsDir(t *testing.T) {
	bin := buildMimic(t)
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "specs", "README.md"), []byte("no specs here"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, exited, err := runAndCapture(t, bin, workdir)
	if exited {
		t.Fatalf("expected server to keep running with an empty specs/ dir, but it exited: %v\noutput:\n%s", err, output)
	}
	if strings.Contains(output, "openapi specs loaded") {
		t.Errorf("expected no openapi adapter to register when specs/ has no spec files, got:\n%s", output)
	}
}

func TestBootLoadsSpecsWhenPresent(t *testing.T) {
	bin := buildMimic(t)
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `openapi: 3.0.3
info:
  title: Smoke Test
  version: 1.0.0
paths:
  /widgets:
    get:
      responses:
        "200":
          description: ok
`
	if err := os.WriteFile(filepath.Join(workdir, "specs", "smoke.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	output, exited, err := runAndCapture(t, bin, workdir)
	if exited {
		t.Fatalf("expected server to keep running with a populated specs/ dir, but it exited: %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "openapi specs loaded") {
		t.Errorf("expected the openapi adapter to register when specs/ has a spec file, got:\n%s", output)
	}
}

func TestUnknownArgExitsNonZero(t *testing.T) {
	bin := buildMimic(t)
	workdir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "bogus")
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "DB_PATH=:memory:")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown argument, got success. output:\n%s", out)
	}
}
