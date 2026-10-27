package buildstamp

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const rev = "778f76aa90091bf10c91bebf740371a6258c54aa"

// stamped is a synthetic binary carrying module information.
func stamped(modified string) []byte {
	info := "path\texample.com/p/cmd/x\nmod\texample.com/p\tv0.0.0\t\n" +
		"build\tGOOS=js\nbuild\tvcs=git\nbuild\tvcs.revision=" + rev + "\n"
	if modified != "" {
		info += "build\tvcs.modified=" + modified + "\n"
	}
	var b bytes.Buffer
	b.WriteString("\x00asm\x01\x00\x00\x00 code ")
	b.Write(infoStart)
	b.WriteString(info)
	b.Write(infoEnd)
	b.WriteString(" more data")
	return b.Bytes()
}

func TestClean(t *testing.T) {
	bi, err := Read(stamped("false"))
	if err != nil {
		t.Fatal(err)
	}
	if bi.Path != "example.com/p/cmd/x" {
		t.Errorf("path %q", bi.Path)
	}
	if err := Clean(bi, ""); err != nil {
		t.Errorf("clean: %v", err)
	}
	if err := Clean(bi, rev); err != nil {
		t.Errorf("clean at rev: %v", err)
	}
	if err := Clean(bi, "0123"); err == nil || !strings.Contains(err.Error(), "want 0123") {
		t.Errorf("other revision: %v", err)
	}
}

func TestDirty(t *testing.T) {
	for _, m := range []string{"true", ""} {
		bi, err := Read(stamped(m))
		if err != nil {
			t.Fatal(err)
		}
		if err := Clean(bi, ""); err == nil || !strings.Contains(err.Error(), "modified") {
			t.Errorf("vcs.modified=%q passed: %v", m, err)
		}
	}
}

func TestUnstamped(t *testing.T) {
	if _, err := Read([]byte("\x00asm no info")); !errors.Is(err, ErrNoInfo) {
		t.Errorf("no info: %v", err)
	}
	open := append(append([]byte{}, infoStart...), bytes.Repeat([]byte("x"), MaxInfo+10)...)
	open = append(open, infoEnd...)
	if _, err := Read(open); err == nil || !strings.Contains(err.Error(), "no end") {
		t.Errorf("unterminated: %v", err)
	}
	bad := append(append(append([]byte{}, infoStart...), "path\tx\nmod\tbroken\n"...), infoEnd...)
	if _, err := Read(bad); err == nil {
		t.Error("malformed module line parsed")
	}
	bi, err := Read(append(append(append([]byte{}, infoStart...), "path\tx\n"...), infoEnd...))
	if err != nil {
		t.Fatal(err)
	}
	if err := Clean(bi, ""); err == nil || !strings.Contains(err.Error(), "vcs.revision") {
		t.Errorf("no vcs stamping passed: %v", err)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.wasm")
	if err := os.WriteFile(f, stamped("false"), 0o644); err != nil {
		t.Fatal(err)
	}
	if bi, err := ReadFile(f); err != nil || bi.Path != "example.com/p/cmd/x" {
		t.Errorf("ReadFile = %v, %v", bi, err)
	}
	if _, err := ReadFile(filepath.Join(dir, "absent")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("absent: %v", err)
	}
	if _, err := ReadFile(dir); err == nil {
		t.Error("a directory was read")
	}
}

// A file over MaxFile is refused without being parsed: a sparse file makes
// the size real without the disk.
func TestReadFileTooLarge(t *testing.T) {
	f := filepath.Join(t.TempDir(), "big")
	fh, err := os.Create(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := fh.Truncate(MaxFile + 1); err != nil {
		fh.Close()
		t.Skip("no sparse files here:", err)
	}
	fh.Close()
	if _, err := ReadFile(f); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("too large: %v", err)
	}
}

// run runs a command in dir, failing the test with its output.
func run(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// buildWasm builds a real js/wasm binary in a repository of its own, once
// from a clean tree and once after an untracked file appears in it — the
// way a CI job that writes its output into the checkout stamps its release.
// It returns both, and the commit.
func buildWasm(t *testing.T) (clean, dirty []byte, commit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/planted\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitEnv := []string{"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}
	run(t, dir, gitEnv, "git", "init", "-q")
	run(t, dir, gitEnv, "git", "add", ".")
	run(t, dir, gitEnv, "git", "commit", "-q", "-m", "planted")
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit = strings.TrimSpace(string(out))
	goEnv := []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=-buildvcs=true"}
	cleanOut := filepath.Join(t.TempDir(), "clean.wasm")
	run(t, dir, goEnv, "go", "build", "-trimpath", "-o", cleanOut, ".")
	if err := os.WriteFile(filepath.Join(dir, "site.html"), []byte("an output written into the checkout"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirtyOut := filepath.Join(t.TempDir(), "dirty.wasm")
	run(t, dir, goEnv, "go", "build", "-trimpath", "-o", dirtyOut, ".")
	if clean, err = os.ReadFile(cleanOut); err != nil {
		t.Fatal(err)
	}
	if dirty, err = os.ReadFile(dirtyOut); err != nil {
		t.Fatal(err)
	}
	return clean, dirty, commit
}

// The real thing, both ways: a wasm built from a clean commit passes at that
// commit, the same commit built with an untracked file in the tree fails —
// and debug/buildinfo cannot read either, which is why this package exists.
func TestRealWasm(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two wasm binaries")
	}
	clean, dirty, commit := buildWasm(t)
	bi, err := Read(clean)
	if err != nil {
		t.Fatal(err)
	}
	if bi.Path != "example.com/planted" {
		t.Errorf("path %q", bi.Path)
	}
	if v, _ := Setting(bi, "GOOS"); v != "js" {
		t.Errorf("GOOS %q", v)
	}
	if err := Clean(bi, commit); err != nil {
		t.Errorf("the clean build failed: %v", err)
	}
	bi, err = Read(dirty)
	if err != nil {
		t.Fatal(err)
	}
	if err := Clean(bi, commit); err == nil || !strings.Contains(err.Error(), "+dirty") {
		t.Errorf("the dirty build passed, or did not say +dirty: %v", err)
	}
}
