package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stampedFile(t *testing.T, modified string) string {
	t.Helper()
	start := "\x30\x77\xaf\x0c\x92\x74\x08\x02\x41\xe1\xc1\x07\xe6\xd6\x18\xe6"
	end := "\xf9\x32\x43\x31\x86\x18\x20\x72\x00\x82\x42\x10\x41\x16\xd8\xf2"
	info := "path\tx/cmd/y\nmod\tx\tv0.4.2\t\nbuild\tvcs=git\nbuild\tvcs.revision=abc\nbuild\tvcs.modified=" + modified + "\n"
	f := filepath.Join(t.TempDir(), "y.wasm")
	if err := os.WriteFile(f, []byte("\x00asm"+start+info+end), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func exec(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := RootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestCheck(t *testing.T) {
	clean, dirty := stampedFile(t, "false"), stampedFile(t, "true")
	if out, err := exec("check", clean); err != nil || !strings.Contains(out, "clean: v0.4.2 at abc") {
		t.Errorf("clean: %v %q", err, out)
	}
	if _, err := exec("check", "-r", "abc", clean); err != nil {
		t.Errorf("clean at abc: %v", err)
	}
	if _, err := exec("check", "--revision", "def", clean); err == nil {
		t.Error("wrong revision passed")
	}
	// Every file is reported, the dirty one among them.
	out, err := exec("check", dirty, clean)
	if err == nil || !strings.Contains(out, "NOT CLEAN") || !strings.Contains(out, "clean: v0.4.2") {
		t.Errorf("dirty: %v %q", err, out)
	}
	if run([]string{"check", dirty}) != 1 || run([]string{"check", clean}) != 0 {
		t.Error("exit status of check")
	}
	if run([]string{"check", filepath.Join(t.TempDir(), "absent")}) != 2 {
		t.Error("a missing file is not status 2")
	}
	if run([]string{"check"}) != 2 || run([]string{"nope"}) != 2 {
		t.Error("usage errors are not status 2")
	}
}

func TestShow(t *testing.T) {
	out, err := exec("show", stampedFile(t, "true"))
	if err != nil || !strings.Contains(out, "\tpath\tx/cmd/y\n") || !strings.Contains(out, "\tbuild\tvcs.modified=true\n") {
		t.Errorf("show: %v %q", err, out)
	}
	if run([]string{"show", filepath.Join(t.TempDir(), "absent")}) != 2 {
		t.Error("show of a missing file")
	}
}

func TestMain_(t *testing.T) {
	code := -1
	osExit = func(c int) { code = c }
	defer func() { osExit = os.Exit }()
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"buildstamp", "check", stampedFile(t, "false")}
	main()
	if code != 0 {
		t.Errorf("main exit = %d", code)
	}
}
