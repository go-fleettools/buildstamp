# buildstamp

Read the Go build information of any binary — **WebAssembly included** — and
fail one that was built from a modified tree.

`go version -m` and `debug/buildinfo` read ELF, Mach-O, PE, XCOFF and Plan 9
files. Given a `.wasm` file they answer `unrecognized file format`. The go
command writes the same module information into every binary, as text between
two 16-byte sentinels (`infoStart` and `infoEnd` in
`cmd/go/internal/modload/build.go`). buildstamp finds that text and hands it to
`runtime/debug.ParseBuildInfo`, so one reader works for every format.

## Why

A release built in CI after something wrote into the checkout is stamped
`vcs.modified=true`. Its main module version reads
`v0.0.0-…-<commit>+dirty`, and it is not the commit it claims to be.
go-wasmcontainers/dashboard v0.4.2 shipped that way: its Pages job wrote
`_site/` inside the work tree. buildstamp is the gate that would have refused
it.

## Command

```
go install github.com/go-fleettools/buildstamp/cmd/buildstamp@latest

buildstamp check [--revision SHA] FILE...   # exit 1 unless every file is clean
buildstamp show FILE...                       # print the build information
```

A file is **clean** when all of these hold:
- it has a `vcs.revision`;
- its `vcs.modified` is `false`;
- with `--revision`, its revision is the commit given.

Exit status:

| Status | Meaning |
|---|---|
| 0 | every file is clean |
| 1 | at least one file is not (every file is still reported) |
| 2 | a file could not be read, or it carries no Go module information |

In a workflow, build **outside** the checkout and gate before you publish:

```yaml
- run: GOOS=js GOARCH=wasm go build -o "$RUNNER_TEMP/site/app.wasm" ./cmd/app
- run: go run github.com/go-fleettools/buildstamp/cmd/buildstamp@v0.1.0 check --revision "$GITHUB_SHA" "$RUNNER_TEMP/site/app.wasm"
```

## Package

```go
import "github.com/go-fleettools/buildstamp"

bi, err := buildstamp.ReadFile("app.wasm") // or buildstamp.Read(bytes)
err = buildstamp.Clean(bi, os.Getenv("GITHUB_SHA"))
rev, _ := buildstamp.Setting(bi, "vcs.revision")
```

The Go version is not part of the module information, so `bi.GoVersion` is
empty.

## Safety

A binary is untrusted input, so every read is bounded:
- the module information is read at most 1 MiB past its start sentinel;
- a file over 1 GiB is refused before it is read;
- `Read` is fuzzed (`FuzzRead`) in CI.

## Tests

- Every statement is covered (`covblocks`).
- The tests build a **real** js/wasm binary twice in a repository of their own: once from a clean commit, and once after an untracked file appears. The clean build must pass at that commit, and the dirty one must fail and say `+dirty`.
- CI runs the tool on its own clean build, and on a planted dirty one, which must exit with status 1.

## License

See [LICENSE](LICENSE).
