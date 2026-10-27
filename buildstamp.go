// Package buildstamp reads the Go build information embedded in a binary of
// any format — js/wasm and wasip1/wasm included — and judges whether it was
// built from a clean commit.
//
// debug/buildinfo reads ELF, Mach-O, PE, XCOFF and Plan 9 files only; given
// a WebAssembly module it answers "unrecognized file format", and so does
// `go version -m`. The go command writes the module information into every
// binary as text between two 16-byte sentinels (infoStart and infoEnd in
// cmd/go/internal/modload/build.go), in the format runtime/debug.ParseBuildInfo
// reads. This package finds the text between the sentinels and hands it to
// the standard parser, which works the same whatever the container format.
//
// The question it exists to answer: a release built in CI after something
// wrote into the checkout (an output directory, a generated file) is stamped
// vcs.modified=true and its main module version gets a "+dirty" suffix — it
// is not the commit it claims to be. Clean reports that.
package buildstamp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// The sentinels around the module information (cmd/go/internal/modload).
var (
	infoStart = []byte("\x30\x77\xaf\x0c\x92\x74\x08\x02\x41\xe1\xc1\x07\xe6\xd6\x18\xe6")
	infoEnd   = []byte("\xf9\x32\x43\x31\x86\x18\x20\x72\x00\x82\x42\x10\x41\x16\xd8\xf2")
)

// MaxInfo bounds the module information: a binary is untrusted input, and a
// start sentinel with no end nearby must not make the parser read megabytes.
const MaxInfo = 1 << 20

// MaxFile bounds what ReadFile reads.
const MaxFile = 1 << 30

// ErrNoInfo is returned for a file that carries no Go module information
// (not a Go binary, or one built without module support).
var ErrNoInfo = errors.New("no Go module information in the file")

// Read returns the build information embedded in bin. The Go version is not
// part of the module information and is left empty.
func Read(bin []byte) (*debug.BuildInfo, error) {
	i := bytes.Index(bin, infoStart)
	if i < 0 {
		return nil, ErrNoInfo
	}
	rest := bin[i+len(infoStart):]
	if len(rest) > MaxInfo {
		rest = rest[:MaxInfo]
	}
	j := bytes.Index(rest, infoEnd)
	if j < 0 {
		return nil, fmt.Errorf("module information has no end within %d bytes", MaxInfo)
	}
	return debug.ParseBuildInfo(string(rest[:j]))
}

// ReadFile reads the build information of the file at path (at most
// MaxFile bytes).
func ReadFile(path string) (*debug.BuildInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > MaxFile {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, MaxFile)
	}
	// Bounded again: a file that grows after Stat is read up to MaxFile.
	bin, err := io.ReadAll(io.LimitReader(f, MaxFile))
	if err != nil {
		return nil, err
	}
	return Read(bin)
}

// Setting returns the value of the build setting key, and whether it is set.
func Setting(bi *debug.BuildInfo, key string) (string, bool) {
	for _, s := range bi.Settings {
		if s.Key == key {
			return s.Value, true
		}
	}
	return "", false
}

// Clean reports an error unless bi was stamped from a version-controlled
// tree with no modification — vcs.revision present and vcs.modified=false —
// at revision want when want is not empty.
func Clean(bi *debug.BuildInfo, want string) error {
	rev, ok := Setting(bi, "vcs.revision")
	if !ok {
		return errors.New("no vcs.revision: built without version-control stamping (-buildvcs=false, or outside a repository)")
	}
	if mod, _ := Setting(bi, "vcs.modified"); mod != "false" {
		return fmt.Errorf("vcs.modified=%q at %s: built from a modified tree (module version %s)", mod, rev, bi.Main.Version)
	}
	if want != "" && rev != want {
		return fmt.Errorf("vcs.revision=%s, want %s", rev, want)
	}
	return nil
}
