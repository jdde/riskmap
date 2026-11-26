// Command riskmap classifies the files of a change with a .riskmap.yml.
//
// See "riskmap help" for the commands.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/jdde/riskmap/internal/config"
	"github.com/jdde/riskmap/internal/eval"
	"github.com/jdde/riskmap/internal/input"
	"github.com/jdde/riskmap/internal/result"
)

// version is replaced at build time by the release workflow.
var version = "dev"

// Exit codes. A caller that only looks at the exit code must never mistake a
// failure for a pass, so only a real result exits with 0.
const (
	exitOK      = 0 // a result was produced / the rule file is valid
	exitInvalid = 1 // no rule file, an invalid rule file or a failing test
	exitUsage   = 2 // wrong arguments or a file could not be read
	exitTooHigh = 3 // check: the change reaches the --fail-on level
)

const usage = `riskmap classifies the files of a change with a .riskmap.yml.

Usage:
  riskmap classify [--format json|text] [--root DIR] [--incomplete] [SOURCE]
      Classify the paths of a change and print the result.
      SOURCE says where the paths come from. Use one of:

      --base REF [--head REF]
          The commits on HEAD (or --head) since it branched off from REF,
          as in a pull request. The rule file is read from the branch
          point, so a change is judged by the rules that were in force
          before it.
      --staged
          What "git commit" would commit now. The rule file is read from
          the last commit. Meant for pre-commit hooks.
      --patch FILE
          A patch written by git. "-" reads the patch from standard input.
          The rule file is read from DIR.
      PATH...
          The given paths. The rule file is read from DIR. Options come
          before the paths. Write "--" before a path that starts with "-".
      (nothing)
          Paths from standard input, separated by NUL bytes, for example:
              git diff --name-only --no-renames -z A B | riskmap classify
          The rule file is read from DIR.

      --incomplete marks the list of paths as possibly incomplete. The
      result is then the strictest one.

  riskmap check [--fail-on LEVEL] [--format json|text] [--root DIR]
                [--incomplete] [SOURCE]
      Like classify, but as a gate: exit with 3 if the risk of the change
      is LEVEL or higher. LEVEL is a value of "risk" (low, medium, high)
      and defaults to high. The JSON result has an additional "check"
      object with the verdict.
      An incomplete list of paths always counts as the highest level.
      Example for a pre-commit hook:
          riskmap check --staged --fail-on high --format text

  riskmap validate [--format json|text] [--root DIR]
      Check the rule file and run its "tests" block.

  riskmap version
  riskmap help

DIR is the directory that holds the .riskmap.yml. It defaults to the
current directory. Paths are relative to it and use "/". A path written
another way ("./a", "a/../b", "/a") is not matched against the rules and
counts as the highest level.

Output:
  Every command prints JSON by default. --format text prints a report
  for people instead.
  Results go to standard output, messages for people to standard error.
  With the exit codes 0, 1 and 3, standard output holds exactly one JSON
  document. With exit code 2 it is empty. The documents are described by
  the JSON Schemas in the "schema" directory of the repository.

Exit codes:
  0  classify: a result with status "ok" or "incomplete" was printed
     check: the risk of the change is below --fail-on
     validate: the rule file is valid and its tests pass
  1  the rule file is missing or invalid, or a test failed
     (the JSON then has the status "no-config", "invalid-config" or
     "tests-failed")
  2  wrong usage, a file could not be read, or git failed
  3  check: the risk of the change is at or above --fail-on
`

func main() {
	// Go note: os.Exit ends the program with the given exit code. All the
	// work happens in run, which is easier to test because it gets its input
	// and output passed in.
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch command, rest := args[0], args[1:]; command {
	case "classify":
		return classify("classify", rest, stdin, stdout, stderr)
	case "check":
		return classify("check", rest, stdin, stdout, stderr)
	case "validate":
		return validate(rest, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "riskmap", version)
		return exitOK
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "riskmap: unknown command %q\n\n%s", command, usage)
		return exitUsage
	}
}

// newFlags prepares the options every command shares.
//
// Go note: a *string is a pointer. flag.Parse fills in the value later, and
// the caller reads it with *root.
func newFlags(name string, stderr io.Writer) (*flag.FlagSet, *string) {
	flags := flag.NewFlagSet("riskmap "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	root := flags.String("root", ".", "directory that holds the .riskmap.yml")
	return flags, root
}

// Values of --format. JSON is the default everywhere: the output is first of
// all the input of other tools.
const (
	formatText = "text"
	formatJSON = "json"
)

func newFormatFlag(flags *flag.FlagSet) *string {
	return flags.String("format", formatJSON, "json or text")
}

// knownFormat reports whether value is allowed for --format, and complains
// on stderr if it is not.
func knownFormat(value string, stderr io.Writer) bool {
	if value == formatText || value == formatJSON {
		return true
	}
	fmt.Fprintf(stderr, "riskmap: --format %q is not one of json, text\n", value)
	return false
}

// loaded is a rule file that was read from disk.
type loaded struct {
	cfg      *config.Config // nil if the file is missing or invalid
	policy   result.Policy
	problems []config.Problem // why the file is invalid
	missing  bool
}

// loadFromDir reads the rule file from the directory root. The error is only
// set when the file exists but cannot be read.
func loadFromDir(root string) (loaded, error) {
	data, err := os.ReadFile(filepath.Join(root, config.FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return loaded{missing: true}, nil
	}
	if err != nil {
		return loaded{}, err
	}
	return parse(data, ""), nil
}

// loadFromGit reads the rule file as it was in a commit.
func loadFromGit(git input.Git, commit string) (loaded, error) {
	data, err := git.ReadFile(commit, config.FileName)
	if errors.Is(err, input.ErrNoFile) {
		return loaded{missing: true}, nil
	}
	if err != nil {
		return loaded{}, err
	}
	return parse(data, commit), nil
}

// parse turns the content of a rule file into a loaded value. ref is the
// commit the content was read from, or empty.
func parse(data []byte, ref string) loaded {
	l := loaded{policy: result.Policy{Ref: ref, Files: []result.PolicyFile{result.NewPolicyFile(config.FileName, data)}}}
	cfg, err := config.Parse(config.FileName, data)
	if err != nil {
		// Parse promises that its error is always an *InvalidError.
		var invalid *config.InvalidError
		errors.As(err, &invalid)
		l.problems = invalid.Problems
		return l
	}
	l.cfg = cfg
	return l
}

// classify implements the commands "classify" and "check". Both read the same
// input; they differ in what they print and in the exit code.
func classify(command string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	check := command == "check"
	flags, root := newFlags(command, stderr)
	failOn := "high"
	if check {
		flags.StringVar(&failOn, "fail-on", failOn, "risk level from which the check fails")
	}
	format := newFormatFlag(flags)
	incomplete := flags.Bool("incomplete", false, "the list of paths may be incomplete")
	base := flags.String("base", "", "git ref the change is based on")
	head := flags.String("head", "", "git ref of the change (default HEAD)")
	staged := flags.Bool("staged", false, "classify what is staged for the next commit")
	patch := flags.String("patch", "", "patch file to read the paths from")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}

	if !knownFormat(*format, stderr) {
		return exitUsage
	}
	asJSON := *format == formatJSON

	// fail reports a problem that is not about the rule file.
	fail := func(message string, args ...any) int {
		fmt.Fprintf(stderr, "riskmap: "+message+"\n", args...)
		return exitUsage
	}

	sources := 0
	for _, used := range []bool{*base != "", *staged, *patch != "", flags.NArg() > 0} {
		if used {
			sources++
		}
	}
	if sources > 1 {
		return fail("use only one of --base, --staged, --patch and PATH arguments")
	}
	if *head != "" && *base == "" {
		return fail("--head needs --base")
	}
	// The levels of "risk" are fixed, so a wrong --fail-on is reported before
	// anything is read.
	if levels := config.RiskLevels(); check && !contains(levels, failOn) {
		return fail("--fail-on %q is not one of %s", failOn, strings.Join(levels, ", "))
	}
	// Go's flag package stops at the first argument that is not an option.
	// In "check a.md --fail-on low" it would take "--fail-on" and "low" as
	// paths and leave the limit at its default, so that is an error.
	if option := optionAfterPath(args, flags.Args()); option != "" {
		return fail("option %q must come before the paths; for a path that starts with \"-\", write \"--\" before the paths", option)
	}

	var (
		paths [][]byte
		l     loaded
		err   error
	)
	git := input.Git{Dir: *root}

	switch {
	case *base != "":
		if *head == "" {
			*head = "HEAD"
		}
		var headCommit, branchPoint string
		if _, err = git.Commit(*base); err != nil {
			return fail("%v", err)
		}
		if headCommit, err = git.Commit(*head); err != nil {
			return fail("%v", err)
		}
		if branchPoint, err = git.MergeBase(*base, headCommit); err != nil {
			return fail("%v", err)
		}
		if paths, err = git.ChangedPaths(branchPoint, headCommit); err != nil {
			return fail("%v", err)
		}
		l, err = loadFromGit(git, branchPoint)

	case *staged:
		var headCommit string
		if headCommit, err = git.Commit("HEAD"); err != nil {
			return fail("--staged needs a repository with at least one commit")
		}
		if paths, err = git.StagedPaths(); err != nil {
			return fail("%v", err)
		}
		l, err = loadFromGit(git, headCommit)

	case *patch != "":
		var data []byte
		if *patch == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(*patch)
		}
		if err != nil {
			return fail("%v", err)
		}
		if paths, err = input.PathsFromPatch(data); err != nil {
			return fail("%s: %v", *patch, err)
		}
		l, err = loadFromDir(*root)

	case flags.NArg() > 0:
		for _, arg := range flags.Args() {
			// On Windows the shell hands over paths with "\". ToSlash turns
			// them into "/" there and does nothing on other systems.
			paths = append(paths, []byte(filepath.ToSlash(arg)))
		}
		l, err = loadFromDir(*root)

	default:
		var data []byte
		if data, err = io.ReadAll(stdin); err != nil {
			return fail("reading standard input: %v", err)
		}
		paths = input.SplitNUL(data)
		l, err = loadFromDir(*root)
	}

	// verdict is the "check" object of the JSON output. Without a usable
	// rule file a check has not passed.
	var verdict *result.Check
	if check {
		verdict = &result.Check{FailOn: failOn}
	}
	switch {
	case err != nil:
		return fail("%v", err)
	case l.missing:
		if asJSON {
			stdout.Write(result.NoConfig(verdict))
		}
		fmt.Fprintf(stderr, "riskmap: no %s found\n", config.FileName)
		return exitInvalid
	case l.cfg == nil:
		if asJSON {
			stdout.Write(result.InvalidConfig(l.policy, l.problems, verdict))
		}
		printProblems(stderr, l.problems)
		return exitInvalid
	}

	r := eval.Evaluate(l.cfg, paths, !*incomplete)
	risk, _ := l.cfg.Dimension("risk") // always declared
	if !check {
		if asJSON {
			stdout.Write(result.Classified(l.cfg, l.policy, r, nil))
		} else {
			report(l.cfg, r, risk, stdout)
		}
		return exitOK
	}

	limit := risk.Rank(failOn) // checked above

	// The verdict: how many paths reach the limit, and whether the change as
	// a whole does. The second is not the same as "at least one path": an
	// empty or incomplete change has no such path and can still fail.
	overall := r.Labels["risk"].Level
	for _, p := range r.Paths {
		if risk.Rank(p.Labels["risk"].Level) >= limit {
			verdict.FailingPaths++
		}
	}
	verdict.Passed = r.Status != eval.StatusIncomplete && risk.Rank(overall) < limit

	if asJSON {
		stdout.Write(result.Classified(l.cfg, l.policy, r, verdict))
	} else {
		report(l.cfg, r, risk, stdout)
	}
	switch {
	case verdict.Passed:
		if !asJSON {
			fmt.Fprintf(stdout, "\nriskmap: ok, below risk %q\n", failOn)
		}
		return exitOK
	case r.Status == eval.StatusIncomplete:
		fmt.Fprintf(stderr, "\nriskmap: FAILED: the list of paths is incomplete, so the change counts as the highest level\n")
	case len(r.Paths) == 0:
		fmt.Fprintf(stderr, "\nriskmap: FAILED: an empty change gets the default, risk %q (--fail-on %s)\n", overall, failOn)
	default:
		fmt.Fprintf(stderr, "\nriskmap: FAILED: %d of %d changed paths at risk %q or higher (--fail-on %s)\n", verdict.FailingPaths, len(r.Paths), failOn, failOn)
	}
	return exitTooHigh
}

// optionAfterPath returns the first PATH argument that looks like an option,
// or "" if there is none. args is everything after the command, paths is what
// the flag package left over. After a "--" every argument is a path, whatever
// it looks like.
func optionAfterPath(args, paths []string) string {
	if before := len(args) - len(paths); before > 0 && args[before-1] == "--" {
		return ""
	}
	for _, path := range paths {
		if strings.HasPrefix(path, "-") {
			return path
		}
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// report prints a classification for a person to read (--format text).
func report(cfg *config.Config, r *eval.Result, risk config.Dimension, stdout io.Writer) {
	// The summary line: the risk first, then every other label.
	summary := []string{"risk: " + r.Labels["risk"].Level}
	for _, d := range cfg.Dimensions {
		if value, present := r.Labels[d.Name]; present && d.Name != "risk" {
			summary = append(summary, d.Name+": "+show(d, value))
		}
	}
	fmt.Fprintln(stdout, strings.Join(summary, "   "))

	// The riskiest paths come first; paths of the same level stay in byte
	// order, which is how Evaluate returns them.
	paths := append([]eval.PathResult(nil), r.Paths...)
	sort.SliceStable(paths, func(i, j int) bool {
		return risk.Rank(paths[i].Labels["risk"].Level) > risk.Rank(paths[j].Labels["risk"].Level)
	})
	if len(paths) > 0 {
		fmt.Fprintln(stdout)
	}
	for _, p := range paths {
		fmt.Fprintf(stdout, "  %-6s  %s\n", p.Labels["risk"].Level, showPath(p.Path))
		for _, b := range p.Because {
			fmt.Fprintf(stdout, "          %s\n", showBecause(b))
		}
	}
}

// showPath writes a path for a person. A path with unusual bytes (a line
// break, an escape sequence) is quoted so that it cannot garble the terminal.
func showPath(path []byte) string {
	text := string(path)
	if quoted := strconv.Quote(text); quoted != `"`+text+`"` {
		return quoted
	}
	return text
}

// showBecause writes one trace entry as "rule: reason".
func showBecause(b eval.Because) string {
	switch b.Kind {
	case eval.KindRule, eval.KindBuiltin:
		return b.Rule + ": " + b.Reason
	case eval.KindDefault:
		return "default: no rule matches this path"
	}
	// The only kind left for a path (see eval.EvaluatePath).
	return "unreadable path (not valid UTF-8, or not written as a/b/c): counts as the highest level"
}

func validate(args []string, stdout, stderr io.Writer) int {
	flags, root := newFlags("validate", stderr)
	format := newFormatFlag(flags)
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if !knownFormat(*format, stderr) {
		return exitUsage
	}
	asJSON := *format == formatJSON
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "riskmap validate: unexpected argument %q\n", flags.Arg(0))
		return exitUsage
	}

	l, err := loadFromDir(*root)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "riskmap: %v\n", err)
		return exitUsage
	case l.missing:
		if asJSON {
			stdout.Write(result.NoConfig(nil))
		}
		fmt.Fprintf(stderr, "riskmap: no %s in %s\n", config.FileName, *root)
		return exitInvalid
	case l.cfg == nil:
		if asJSON {
			stdout.Write(result.InvalidConfig(l.policy, l.problems, nil))
		}
		printProblems(stderr, l.problems)
		return exitInvalid
	}

	failures := runTests(l.cfg)
	if asJSON {
		var list []result.TestFailure
		for _, f := range failures {
			list = append(list, result.TestFailure{
				File: l.cfg.File, Line: f.test.Line, Path: f.test.Path, Dimension: f.dimension.Name,
				Expected: result.ValueToJSON(f.dimension, f.want),
				Actual:   result.ValueToJSON(f.dimension, f.have),
			})
		}
		stdout.Write(result.Validated(l.cfg, l.policy, list))
	}
	for _, f := range failures {
		fmt.Fprintf(stderr, "%s:%d: test for %q: %s is %s, expected %s\n", l.cfg.File, f.test.Line,
			f.test.Path, f.dimension.Name, show(f.dimension, f.have), show(f.dimension, f.want))
	}
	if len(failures) > 0 {
		fmt.Fprintf(stderr, "%d of %d tests failed\n", len(failures), len(l.cfg.Tests))
		return exitInvalid
	}
	if !asJSON {
		fmt.Fprintf(stdout, "%s is valid: %d rules, %d tests passed\n", config.FileName, len(l.cfg.Rules), len(l.cfg.Tests))
	}
	return exitOK
}

// testFailure is one expectation of the "tests" block that did not hold.
type testFailure struct {
	test       config.Test
	dimension  config.Dimension
	have, want config.Value
}

// runTests runs the "tests" block of a rule file (section 8 of the
// specification) and returns one entry per failing expectation.
//
// An expectation is a subset: dimensions it names must match exactly,
// dimensions it does not name are not checked.
func runTests(cfg *config.Config) []testFailure {
	var failures []testFailure
	for _, test := range cfg.Tests {
		got := eval.EvaluatePath(cfg, []byte(test.Path))
		for _, d := range cfg.Dimensions {
			want, named := test.Expect[d.Name]
			if !named {
				continue
			}
			// DeepEqual compares the lists inside the values, too. Both
			// sides hold sorted lists, so the order does not matter.
			if have := got.Labels[d.Name]; !reflect.DeepEqual(normalize(have), normalize(want)) {
				failures = append(failures, testFailure{test: test, dimension: d, have: have, want: want})
			}
		}
	}
	return failures
}

// normalize makes an empty list and a missing list compare as equal.
func normalize(v config.Value) config.Value {
	if len(v.Set) == 0 {
		v.Set = nil
	}
	return v
}

// show formats a value for a message.
func show(d config.Dimension, v config.Value) string {
	switch d.Kind {
	case config.Set:
		return "[" + strings.Join(v.Set, ", ") + "]"
	case config.Flag:
		return fmt.Sprint(v.Flag)
	}
	if v.Level == "" {
		return "not set"
	}
	return v.Level
}

func printProblems(stderr io.Writer, problems []config.Problem) {
	for _, p := range problems {
		fmt.Fprintln(stderr, p)
	}
}
