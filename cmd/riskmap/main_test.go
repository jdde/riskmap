package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

const example = "../../tests/examples/nextjs-web"

// invoke runs the CLI in the test process and returns what it printed.
func invoke(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, err bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &err)
	return code, out.String(), err.String()
}

// write creates a directory with the given rule file and returns its path.
func write(t *testing.T, rules string) string {
	t.Helper()
	// Go note: t.TempDir is removed automatically when the test ends.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".riskmap.yml"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

type output struct {
	Status string         `json:"status"`
	Labels map[string]any `json:"labels"`
	Paths  []struct {
		Path string `json:"path"`
	} `json:"paths"`
	Errors []map[string]any `json:"errors"`
}

func decode(t *testing.T, stdout string) output {
	t.Helper()
	var doc output
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	return doc
}

func TestClassifyArguments(t *testing.T) {
	code, stdout, stderr := invoke("", "classify", "--root", example, "README.md", "src/components/Button.tsx")
	if code != exitOK || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	doc := decode(t, stdout)
	if doc.Status != "ok" || doc.Labels["risk"] != "medium" || len(doc.Paths) != 2 {
		t.Errorf("unexpected result: %+v", doc)
	}
}

func TestClassifyStdin(t *testing.T) {
	// NUL-separated, with a final NUL as git writes it. The second path
	// contains a line break, which must not split it.
	code, stdout, _ := invoke("README.md\x00docs/a\nb.md\x00", "classify", "--root", example)
	doc := decode(t, stdout)
	if code != exitOK || len(doc.Paths) != 2 || doc.Paths[1].Path != "docs/a\nb.md" || doc.Labels["risk"] != "low" {
		t.Errorf("exit %d, result %+v", code, doc)
	}
}

func TestClassifyEmptyStdin(t *testing.T) {
	code, stdout, _ := invoke("", "classify", "--root", example)
	doc := decode(t, stdout)
	if code != exitOK || doc.Status != "ok" || doc.Labels["risk"] != "high" || len(doc.Paths) != 0 {
		t.Errorf("exit %d, result %+v", code, doc)
	}
}

func TestClassifyIncomplete(t *testing.T) {
	code, stdout, _ := invoke("", "classify", "--incomplete", "--root", example, "README.md")
	doc := decode(t, stdout)
	if code != exitOK || doc.Status != "incomplete" || doc.Labels["risk"] != "high" || doc.Labels["protected"] != true {
		t.Errorf("exit %d, result %+v", code, doc)
	}
}

func TestClassifyWithoutRuleFile(t *testing.T) {
	code, stdout, stderr := invoke("", "classify", "--root", t.TempDir(), "README.md")
	doc := decode(t, stdout)
	if code != exitInvalid || doc.Status != "no-config" || doc.Labels != nil {
		t.Errorf("exit %d, result %+v", code, doc)
	}
	if !strings.Contains(stderr, "no .riskmap.yml") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestClassifyWithInvalidRuleFile(t *testing.T) {
	dir := write(t, "version: 1\ndefault: { risk: low }\nrules:\n  - match: [\"!a\"]\n    labels: { risk: low }\n    reason: x\n")
	code, stdout, stderr := invoke("", "classify", "--root", dir, "README.md")
	doc := decode(t, stdout)
	if code != exitInvalid || doc.Status != "invalid-config" || doc.Labels != nil || len(doc.Errors) != 1 {
		t.Errorf("exit %d, result %+v", code, doc)
	}
	if !strings.Contains(stderr, ".riskmap.yml:4:13: invalid glob") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestValidate(t *testing.T) {
	code, stdout, stderr := invoke("", "validate", "--format", "text", "--root", example)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, "is valid: 8 rules, 9 tests passed") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestValidateFailingTest(t *testing.T) {
	dir := write(t, `version: 1
dimensions:
  domain: { type: set }
default: { risk: high }
rules:
  - id: docs
    match: ["docs/**"]
    labels: { risk: low, domain: [docs] }
    reason: "Documentation"
tests:
  - path: "docs/a.md"
    expect: { risk: low }
  - path: "docs2/a.md"
    expect: { risk: low, domain: [] }
  - path: "docs/b.md"
    expect: { domain: [] }
`)
	code, stdout, stderr := invoke("", "validate", "--format", "text", "--root", dir)
	want := `.riskmap.yml:13: test for "docs2/a.md": risk is high, expected low
.riskmap.yml:15: test for "docs/b.md": domain is [docs], expected []
2 of 3 tests failed
`
	if code != exitInvalid || stdout != "" || stderr != want {
		t.Errorf("exit %d, stdout %q, stderr:\n%s\nwant:\n%s", code, stdout, stderr, want)
	}
}

func TestValidateInvalid(t *testing.T) {
	code, _, stderr := invoke("", "validate", "--root", write(t, "version: 1\n"))
	if code != exitInvalid || !strings.Contains(stderr, `missing key "default"`) || !strings.Contains(stderr, `missing key "rules"`) {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestUsage(t *testing.T) {
	cases := []struct {
		args []string
		code int
	}{
		{nil, exitUsage},
		{[]string{"frobnicate"}, exitUsage},
		{[]string{"classify", "--nope"}, exitUsage},
		{[]string{"validate", "extra"}, exitUsage},
		{[]string{"help"}, exitOK},
		{[]string{"version"}, exitOK},
	}
	for _, c := range cases {
		if code, _, _ := invoke("", c.args...); code != c.code {
			t.Errorf("riskmap %v: exit %d, want %d", c.args, code, c.code)
		}
	}
}

func TestClassifyPatch(t *testing.T) {
	patch := "../../tests/acceptance/nextjs-web/rename-out-of-auth.patch"
	code, stdout, stderr := invoke("", "classify", "--root", example, "--patch", patch)
	doc := decode(t, stdout)
	if code != exitOK || len(doc.Paths) != 2 || doc.Labels["risk"] != "high" {
		t.Errorf("exit %d, stderr %q, result %+v", code, stderr, doc)
	}

	// "-" reads the patch from standard input.
	data, err := os.ReadFile(patch)
	if err != nil {
		t.Fatal(err)
	}
	_, fromStdin, _ := invoke(string(data), "classify", "--root", example, "--patch", "-")
	if fromStdin != stdout {
		t.Errorf("patch from stdin gives a different result:\n%s", fromStdin)
	}
}

func TestClassifyBrokenPatch(t *testing.T) {
	file := filepath.Join(t.TempDir(), "broken.patch")
	if err := os.WriteFile(file, []byte("this is not a patch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No result at all is better than a result for the wrong paths.
	code, stdout, stderr := invoke("", "classify", "--root", example, "--patch", file)
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, "not a git patch") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestClassifySourcesExcludeEachOther(t *testing.T) {
	for _, args := range [][]string{
		{"classify", "--staged", "README.md"},
		{"classify", "--base", "main", "--staged"},
		{"classify", "--patch", "x.patch", "--base", "main"},
		{"classify", "--head", "main"},
	} {
		if code, stdout, _ := invoke("", args...); code != exitUsage || stdout != "" {
			t.Errorf("riskmap %v: exit %d, stdout %q", args, code, stdout)
		}
	}
}

// gitRepo creates a repository with a rule file and one commit.
func gitRepo(t *testing.T) (dir string, git func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir = write(t, `version: 1
default: { risk: high }
rules:
  - id: docs
    match: ["docs/**"]
    labels: { risk: low }
    reason: "Documentation"
`)
	git = func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("add", "--all")
	git("commit", "--quiet", "-m", "base")
	return dir, git
}

func TestClassifyBase(t *testing.T) {
	dir, git := gitRepo(t)
	git("checkout", "--quiet", "-b", "feature")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "a.md"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "--all")
	git("commit", "--quiet", "-m", "docs")

	code, stdout, stderr := invoke("", "classify", "--root", dir, "--base", "main")
	doc := decode(t, stdout)
	if code != exitOK || doc.Labels["risk"] != "low" || len(doc.Paths) != 1 || doc.Paths[0].Path != "docs/a.md" {
		t.Fatalf("exit %d, stderr %q, result %+v", code, stderr, doc)
	}
	// The result names the commit the rules were read from.
	if !strings.Contains(stdout, `"ref": "`) {
		t.Errorf("no policy.ref in:\n%s", stdout)
	}

	// The change now lowers the default and edits the rule file. It is still
	// judged by the rules from the base, and the rule file protects itself.
	lowered := "version: 1\ndefault: { risk: low }\nrules: []\n"
	if err := os.WriteFile(filepath.Join(dir, ".riskmap.yml"), []byte(lowered), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "--all")
	git("commit", "--quiet", "-m", "lower the default")

	code, stdout, _ = invoke("", "classify", "--root", dir, "--base", "main", "--head", "feature")
	doc = decode(t, stdout)
	if code != exitOK || doc.Labels["risk"] != "high" || len(doc.Paths) != 3 {
		t.Errorf("exit %d, result %+v", code, doc)
	}

	code, stdout, stderr = invoke("", "classify", "--root", dir, "--base", "no-such-branch")
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, "not a commit") {
		t.Errorf("unknown base: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestClassifyStaged(t *testing.T) {
	dir, git := gitRepo(t)

	// Nothing staged: an empty change, which gets the default.
	code, stdout, _ := invoke("", "classify", "--root", dir, "--staged")
	doc := decode(t, stdout)
	if code != exitOK || doc.Labels["risk"] != "high" || len(doc.Paths) != 0 {
		t.Errorf("nothing staged: exit %d, result %+v", code, doc)
	}

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "a.md"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unstaged.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "docs/a.md")

	code, stdout, _ = invoke("", "classify", "--root", dir, "--staged")
	doc = decode(t, stdout)
	if code != exitOK || doc.Labels["risk"] != "low" || len(doc.Paths) != 1 {
		t.Errorf("exit %d, result %+v", code, doc)
	}
}

// A repository whose base commit has no rule file gives "no-config", even if
// the change adds one.
func TestClassifyBaseWithoutRuleFile(t *testing.T) {
	dir, git := gitRepo(t)
	git("rm", "--quiet", ".riskmap.yml")
	git("commit", "--quiet", "-m", "remove the rule file")
	git("checkout", "--quiet", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, ".riskmap.yml"), []byte("version: 1\ndefault: { risk: low }\nrules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "--all")
	git("commit", "--quiet", "-m", "add a rule file")

	code, stdout, _ := invoke("", "classify", "--root", dir, "--base", "main")
	if doc := decode(t, stdout); code != exitInvalid || doc.Status != "no-config" {
		t.Errorf("exit %d, result %+v", code, doc)
	}
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		code       int
		wantStdout string
		wantStderr string
	}{
		{"below the limit", []string{"README.md", "src/components/Button.tsx"}, exitOK, "  medium  src/components/Button.tsx\n", ""},
		{"at the limit", []string{"README.md", ".github/workflows/ci.yml"}, exitTooHigh, "  high    .github/workflows/ci.yml\n          ci: ", "FAILED: 1 of 2 changed paths"},
		{"lower limit", []string{"--fail-on", "medium", "src/components/Button.tsx"}, exitTooHigh, "risk: medium", `at risk "medium" or higher`},
		{"default applies", []string{"unknown.txt"}, exitTooHigh, "default: no rule matches this path", "FAILED"},
		{"incomplete", []string{"--incomplete", "README.md"}, exitTooHigh, "risk: high", "incomplete"},
		{"unknown level", []string{"--fail-on", "critical", "README.md"}, exitUsage, "", "is not one of low, medium, high"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"check", "--format", "text", "--root", example}, c.args...)
			code, stdout, stderr := invoke("", args...)
			if code != c.code {
				t.Errorf("exit %d, want %d\nstdout: %s\nstderr: %s", code, c.code, stdout, stderr)
			}
			if !strings.Contains(stdout, c.wantStdout) {
				t.Errorf("stdout does not contain %q:\n%s", c.wantStdout, stdout)
			}
			if !strings.Contains(stderr, c.wantStderr) || (c.wantStderr == "" && stderr != "") {
				t.Errorf("stderr does not contain %q:\n%s", c.wantStderr, stderr)
			}
		})
	}
}

func TestCheckEmptyChange(t *testing.T) {
	code, _, stderr := invoke("", "check", "--root", example)
	if code != exitTooHigh || !strings.Contains(stderr, "an empty change gets the default") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestCheckWithoutRuleFile(t *testing.T) {
	code, stdout, stderr := invoke("", "check", "--format", "text", "--root", t.TempDir(), "a.txt")
	if code != exitInvalid || stdout != "" || !strings.Contains(stderr, "no .riskmap.yml found") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestCheckQuotesUnusualPaths(t *testing.T) {
	code, stdout, _ := invoke("", "check", "--format", "text", "--root", example, "docs/a\nb.md")
	if code != exitOK || !strings.Contains(stdout, `"docs/a\nb.md"`) {
		t.Errorf("exit %d, stdout:\n%s", code, stdout)
	}
}

// TestInputErrors covers input that cannot be read. None of these may print a
// result or exit with 0.
func TestInputErrors(t *testing.T) {
	// A rule file that exists but cannot be read: a directory of that name.
	unreadable := t.TempDir()
	if err := os.Mkdir(filepath.Join(unreadable, ".riskmap.yml"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir, git := gitRepo(t)
	git("checkout", "--quiet", "--orphan", "unrelated")
	git("commit", "--quiet", "--allow-empty", "-m", "no common history")
	empty := t.TempDir() // a directory that is not a git repository

	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"classify: unreadable rule file", []string{"classify", "--root", unreadable, "a.txt"}, exitUsage, "riskmap: "},
		{"validate: unreadable rule file", []string{"validate", "--root", unreadable}, exitUsage, "riskmap: "},
		{"validate: no rule file", []string{"validate", "--format", "text", "--root", empty}, exitInvalid, "no .riskmap.yml in"},
		{"validate: unknown option", []string{"validate", "--bogus"}, exitUsage, "bogus"},
		{"unknown head", []string{"classify", "--root", dir, "--base", "main", "--head", "no-such-ref"}, exitUsage, "is not a commit"},
		{"no common ancestor", []string{"classify", "--root", dir, "--base", "main"}, exitUsage, "no common ancestor"},
		{"staged outside a repository", []string{"classify", "--root", empty, "--staged"}, exitUsage, "--staged needs a repository"},
		{"missing patch file", []string{"classify", "--root", example, "--patch", filepath.Join(empty, "no.patch")}, exitUsage, "no.patch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, stdout, stderr := invoke("", c.args...)
			if code != c.code || stdout != "" || !strings.Contains(stderr, c.want) {
				t.Errorf("exit %d (want %d), stdout %q, stderr %q (want %q)", code, c.code, stdout, stderr, c.want)
			}
		})
	}
}

func TestClassifyUnreadableStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// iotest.ErrReader is a reader that always fails.
	code := run([]string{"classify", "--root", example}, iotest.ErrReader(errors.New("broken pipe")), &stdout, &stderr)
	if code != exitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "reading standard input: broken pipe") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestCheckUnreadablePath(t *testing.T) {
	code, stdout, _ := invoke("", "check", "--format", "text", "--root", example, "docs/\xff.md")
	if code != exitTooHigh || !strings.Contains(stdout, "unreadable path: counts as the highest level") {
		t.Errorf("exit %d, stdout:\n%s", code, stdout)
	}
}

func TestValidateReportsMissingLevel(t *testing.T) {
	dir := write(t, `version: 1
dimensions:
  data: { type: ordered, values: [internal, personal] }
default: { risk: high }
rules:
  - id: docs
    match: ["docs/**"]
    labels: { risk: low }
    reason: "Documentation"
tests:
  - path: "docs/a.md"
    expect: { data: personal }
`)
	code, _, stderr := invoke("", "validate", "--root", dir)
	if code != exitInvalid || !strings.Contains(stderr, "data is not set, expected personal") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

// TestDamagedRepository removes single objects from the git database. git
// then fails halfway through, and the CLI must report that instead of
// printing a result.
func TestDamagedRepository(t *testing.T) {
	// remove deletes the object that a git expression such as "HEAD^{tree}"
	// names. Git stores a new object in .git/objects/<2 hex>/<rest of hash>.
	remove := func(t *testing.T, dir, expression string) {
		t.Helper()
		out, err := exec.Command("git", "-C", dir, "rev-parse", expression).Output()
		if err != nil {
			t.Fatal(err)
		}
		hash := strings.TrimSpace(string(out))
		if err := os.Remove(filepath.Join(dir, ".git", "objects", hash[:2], hash[2:])); err != nil {
			t.Fatal(err)
		}
	}
	// feature returns a repository with one commit on top of main.
	feature := func(t *testing.T) string {
		dir, git := gitRepo(t)
		git("checkout", "--quiet", "-b", "feature")
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", "--all")
		git("commit", "--quiet", "-m", "change")
		return dir
	}

	t.Run("file list of the change is gone", func(t *testing.T) {
		dir := feature(t)
		remove(t, dir, "HEAD^{tree}")
		code, stdout, stderr := invoke("", "classify", "--root", dir, "--base", "main")
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "git diff") {
			t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
	t.Run("rule file of the base is gone", func(t *testing.T) {
		dir := feature(t)
		remove(t, dir, "main:.riskmap.yml")
		code, stdout, stderr := invoke("", "classify", "--root", dir, "--base", "main")
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "git show") {
			t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
	t.Run("staging area is damaged", func(t *testing.T) {
		dir, _ := gitRepo(t)
		if err := os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("not an index"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke("", "classify", "--root", dir, "--staged")
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "git diff") {
			t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
}
