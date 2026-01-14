package gab_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	codalotlcli "github.com/codalotl/codalotl/internal/cli"
)

func TestMain(m *testing.M) {
	// Ensure all tests (and any subprocesses spawned by the CLI) run as if invoked
	// from the module root (the directory containing go.mod).
	wd, err := os.Getwd()
	if err != nil {
		panic("getwd: " + err.Error())
	}
	modRoot, err := moduleRootDirFrom(wd)
	if err != nil {
		panic(err.Error())
	}
	if err := os.Chdir(modRoot); err != nil {
		panic("chdir to module root: " + err.Error())
	}

	os.Exit(m.Run())
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	code, err := codalotlcli.Run(args, &codalotlcli.RunOptions{
		Out: &outBuf,
		Err: &errBuf,
	})
	if code != 0 || err != nil {
		t.Fatalf("cli.Run(%q) = (%d, %v), want (0, nil); stdout=%q; stderr=%q", args, code, err, outBuf.String(), errBuf.String())
	}
	return outBuf.String(), errBuf.String()
}

func packageDir(t *testing.T) string {
	t.Helper()

	// Resolve fixtures relative to this test file's directory, not the process CWD.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Dir(thisFile)
}

func requireFixtureMatch(t *testing.T, relPath string, got string) {
	t.Helper()

	fixturePath := filepath.Join(packageDir(t), relPath)

	b, err := os.ReadFile(fixturePath)
	if err == nil && got == string(b) {
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			// Mismatch.
		} else {
			t.Fatalf("read fixture %q: %v", fixturePath, err)
		}
	}

	if mkErr := os.MkdirAll(filepath.Dir(fixturePath), 0o755); mkErr != nil {
		t.Fatalf("create fixture dir %q: %v", filepath.Dir(fixturePath), mkErr)
	}
	if wErr := os.WriteFile(fixturePath, []byte(got), 0o644); wErr != nil {
		t.Fatalf("write fixture %q: %v", fixturePath, wErr)
	}

	if err == nil {
		t.Fatalf("fixture %q did not match current output; updated it. Re-run tests.", fixturePath)
	}
	t.Fatalf("fixture %q did not exist; recorded current output. Re-run tests.", fixturePath)
}

func moduleRootDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	modRoot, err := moduleRootDirFrom(wd)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return modRoot
}

func moduleRootDirFrom(wd string) (string, error) {
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find go.mod walking up from " + strconv.Quote(wd))
		}
		dir = parent
	}
}

func primeGoTestCache(t *testing.T, pkgRel string) {
	t.Helper()

	modRoot := moduleRootDir(t)
	arg := "./" + filepath.ToSlash(pkgRel)

	cmd := exec.Command("go", "test", arg)
	cmd.Dir = modRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("priming cache failed: %v\ncmd: (dir=%q) go test %s\noutput:\n%s", err, modRoot, arg, string(out))
	}
}

func TestCLI_HelpShortFlagWorks(t *testing.T) {
	stdout, _ := runCLI(t, "codalotl", "-h")

	// Don't lock in exact help text (it's naturally more likely to change / reflow).
	if !strings.Contains(stdout, "context") {
		t.Fatalf("help output missing expected content; got=%q", stdout)
	}
}

func TestCLI_ContextPublic_InternalGotypes_Exact(t *testing.T) {
	stdout, stderr := runCLI(t, "codalotl", "context", "public", "internal/gotypes")
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	requireFixtureMatch(t, filepath.Join("testdata", "context_public_internal_gotypes.txt"), stdout)
}

func TestCLI_ContextInitial_InternalGotypes_Exact(t *testing.T) {
	// initialcontext embeds `go test` output; prime the cache so the output is stable (no timing info).
	primeGoTestCache(t, "internal/gotypes")

	stdout, stderr := runCLI(t, "codalotl", "context", "initial", "internal/gotypes")
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	requireFixtureMatch(t, filepath.Join("testdata", "context_initial_internal_gotypes.txt"), stdout)
}
