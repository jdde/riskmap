package input

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// repo is a throw-away git repository for a test.
type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := repo{t: t, dir: t.TempDir()}
	r.git("init", "--quiet", "--initial-branch=main")
	return r
}

// git runs a git command in the repository, independent of the settings of
// the person running the tests.
func (r repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{
		"-C", r.dir,
		"-c", "user.name=Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.autocrlf=false",
	}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r repo) write(name, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r repo) commit(message string) string {
	r.t.Helper()
	r.git("add", "--all")
	r.git("commit", "--quiet", "--allow-empty", "-m", message)
	return r.git("rev-parse", "HEAD")
}

func sorted(paths [][]byte) []string {
	out := []string{}
	for _, p := range paths {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}

func TestGitChangedPaths(t *testing.T) {
	r := newRepo(t)
	r.write(".riskmap.yml", "version: 1\n")
	r.write("src/keep.ts", "keep\n")
	r.write("src/old name.ts", "a long enough text to be detected as a rename\n")
	r.write("src/delete.ts", "delete\n")
	r.write("src/change.ts", "before\n")
	base := r.commit("base")

	// A name with a character outside ASCII and one with a line break. Git
	// would print the first one quoted and escaped without -z. (The name has
	// no accents on purpose: macOS stores those in a different byte form.)
	r.write("docs/日本 語.md", "new\n")
	want := []string{
		"docs/new.md",     // new path of the rename
		"docs/日本 語.md",    // not quoted
		"src/change.ts",   //
		"src/delete.ts",   // deleted files count
		"src/old name.ts", // old path of the rename
	}
	if runtime.GOOS != "windows" { // Windows does not allow this file name
		r.write("docs/line\nbreak.md", "new\n")
		want = append(want, "docs/line\nbreak.md") // not split at the line break
	}
	r.git("mv", "src/old name.ts", "docs/new.md")
	r.git("rm", "--quiet", "src/delete.ts")
	r.write("src/change.ts", "after\n")
	head := r.commit("change")

	g := Git{Dir: r.dir}
	got, err := g.ChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(sorted(got), want) {
		t.Errorf("got %q, want %q", sorted(got), want)
	}

	// The rule file is read from a commit, not from the working directory.
	r.write(".riskmap.yml", "version: 2\n")
	content, err := g.ReadFile(base, ".riskmap.yml")
	if err != nil || string(content) != "version: 1\n" {
		t.Errorf("ReadFile = %q, %v", content, err)
	}
	if _, err := g.ReadFile(base, "missing.yml"); !errors.Is(err, ErrNoFile) {
		t.Errorf("ReadFile of a missing file: %v, want ErrNoFile", err)
	}
}

func TestGitMergeBase(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	fork := r.commit("fork point")

	r.git("checkout", "--quiet", "-b", "feature")
	r.write("feature.txt", "feature\n")
	r.commit("on feature")

	r.git("checkout", "--quiet", "main")
	r.write("main.txt", "main\n")
	r.commit("on main")

	g := Git{Dir: r.dir}
	got, err := g.MergeBase("main", "feature")
	if err != nil || got != fork {
		t.Fatalf("MergeBase = %q, %v, want %q", got, err, fork)
	}
	// Compared with the fork point, the feature branch changed one file.
	// What happened on main in the meantime does not count.
	paths, err := g.ChangedPaths(got, "feature")
	if err != nil || !reflect.DeepEqual(sorted(paths), []string{"feature.txt"}) {
		t.Errorf("ChangedPaths = %q, %v", sorted(paths), err)
	}

	if _, err := g.Commit("no-such-branch"); err == nil {
		t.Error("Commit of an unknown ref succeeded")
	}
	if _, err := g.Commit("--version"); err == nil {
		t.Error("Commit accepted an option as ref")
	}
}

func TestGitStagedPaths(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	r.commit("base")

	r.write("a.txt", "changed and staged\n")
	r.write("new.txt", "new and staged\n")
	r.git("add", "a.txt", "new.txt")
	r.write("b.txt", "changed but not staged\n")
	r.write("untracked.txt", "not added\n")

	got, err := Git{Dir: r.dir}.StagedPaths()
	if err != nil || !reflect.DeepEqual(sorted(got), []string{"a.txt", "new.txt"}) {
		t.Errorf("StagedPaths = %q, %v", sorted(got), err)
	}
}

// With Dir set to a subdirectory, paths are relative to it and changes
// outside of it are not listed.
func TestGitSubdirectory(t *testing.T) {
	r := newRepo(t)
	r.write("apps/web/.riskmap.yml", "version: 1\n")
	r.write("apps/web/src/a.ts", "a\n")
	r.write("other/b.ts", "b\n")
	base := r.commit("base")

	r.write("apps/web/src/a.ts", "changed\n")
	r.write("other/b.ts", "changed\n")
	head := r.commit("change")

	g := Git{Dir: filepath.Join(r.dir, "apps", "web")}
	paths, err := g.ChangedPaths(base, head)
	if err != nil || !reflect.DeepEqual(sorted(paths), []string{"src/a.ts"}) {
		t.Errorf("ChangedPaths = %q, %v", sorted(paths), err)
	}
	content, err := g.ReadFile(base, ".riskmap.yml")
	if err != nil || string(content) != "version: 1\n" {
		t.Errorf("ReadFile = %q, %v", content, err)
	}
}

func TestGitErrors(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "a\n")
	head := r.commit("first")
	git := Git{Dir: r.dir}

	if _, err := git.ChangedPaths("no-such-ref", head); err == nil || !strings.Contains(err.Error(), "git diff:") {
		t.Errorf("ChangedPaths with an unknown ref: got %v, want git's message", err)
	}
	if _, err := git.ReadFile("no-such-ref", "a.txt"); err == nil || errors.Is(err, ErrNoFile) {
		t.Errorf("ReadFile with an unknown commit: got %v, want a git error", err)
	}

	// A branch without a common ancestor, as in a shallow clone.
	r.git("checkout", "--quiet", "--orphan", "other")
	r.write("b.txt", "b\n")
	other := r.commit("unrelated")
	if _, err := git.MergeBase(head, other); err == nil || !strings.Contains(err.Error(), "no common ancestor") {
		t.Errorf("MergeBase of unrelated commits: got %v", err)
	}
}
