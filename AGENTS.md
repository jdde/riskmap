# AGENTS.md

Orientation for agents and people who work in this repository.

## What Riskmap is

Riskmap is meant to become an open, tool-neutral format (`.riskmap.yml`) with which a repository classifies its files by risk, using globs. Tools derive from it how thoroughly a change is checked: PR briefing, review depth, merge gate.

The format only classifies. What follows from the labels is decided by the tool that reads the result. Who reviews is not part of the format in v1.

## State

The repository holds the specification, the CLI in Go, example projects with expected results, and the survey of prior art. The CLI can validate a rule file (`riskmap validate`), classify a change (`riskmap classify`) and gate on it (`riskmap check`). Every command prints JSON by default and a report for people with `--format text`. Partial configs and distribution beyond GitHub releases are still missing; see "Implementing the CLI". There is no conformance suite yet; section 12 of the spec describes its minimum scope.

Everything in the repository is written in English: documents, code, comments, error messages.

## Files

| File | Content | Role |
|---|---|---|
| `README.md` | what Riskmap is, quick start, rule basics, commands | entry point for users, not normative |
| `SPEC.md` | specification v1, draft 0.2 | normative |
| `docs/prior-art.md` | inventory of all surveyed implementations, findings in sections F and G | research |
| `docs/demo.gif`, `docs/demo/` | the animation in the README and the scripts that re-create it | `sh docs/demo/record.sh` |
| `go.mod`, `go.sum`, `cmd/`, `internal/` | Go source of the CLI | implementation |
| `.github/workflows/ci.yml` | on every pull request and on pushes to `main` and `develop`: format, `go vet`, tests, coverage check, trial build for six targets | CI |
| `.github/workflows/gitflow.yml` | checks source and target branch of a pull request | CI |
| `.github/workflows/release-prepare.yml` | started by hand by the owner: creates `release/<version>` from `develop` and the pull request into `main` | release, see `CONTRIBUTING.md` |
| `.github/workflows/release-publish.yml` | when a `release/*` or `hotfix/*` pull request is merged into `main`: tag, binaries for six targets, checksums, GitHub release, pull request back into `develop` | release |
| `CONTRIBUTING.md` | branch model (gitflow) and the checks on a pull request | read before opening a pull request |
| `scripts/coverage.sh` | measures test coverage, fails below 99 % | see "Test coverage" |
| `schema/` | JSON Schemas of the output of `classify`, `check` and `validate` | contract for tools that read the output |
| `tests/examples/<example>/` | example projects (Next.js, Laravel, Flutter), each with a `.riskmap.yml` | illustration and test basis |
| `tests/acceptance/` | expected results per example and one `.patch` file per case | acceptance test of the CLI |
| `tests/acceptance_test.go` | Go test that runs the cases | runs with `go test ./...` |
| `tests/make-patches.py` | regenerates the patches | see `tests/README.md` |
| `tests/README.md` | format of the cases, comparison rule, maintenance | read first before changing anything under `tests/` |

Reading order to get started: README, then the spec, then sections F and G of the inventory.

Removed on 2026-11-23, still in the git history (commit `bb5d6a8`): the German originals, the format sketch `riskmap-formatskizze.md` (its content went into the README) and the 53 per-project profiles `riskmap-steckbriefe/` with `path:line` evidence for the inventory.

## Conventions

- **IDs:** `A1`–`A14` are reusable tools and actions, `B1`–`B40` in-repo implementations, `C` products, `D` documented pitfalls. A reference such as "(B13)" in the spec points to the row with that ID in `docs/prior-art.md`.
- **Normative words:** MUST, MUST NOT, SHOULD and MAY are normative in the spec.
- **Section numbers of the spec** are cited in code comments and other documents. When rearranging, update the references.
- **Evidence:** A statement about someone else's implementation needs a source. Where the documentation and the code of an implementation disagree, the code counts.
- **README examples show real output.** When the output of the CLI changes, update the README and re-create the GIF.

## Branches

The repository follows gitflow; `CONTRIBUTING.md` has the details. Work on `feature/<name>` branches created from `develop` and open pull requests into `develop`. Only `release/*` and `hotfix/*` go into `main`. Never commit to `main` or `develop` directly.

## Acceptance tests

`tests/` holds three example projects and their expected classifications. The rule behind it: as long as the `.riskmap.yml` of an example does not change in substance, the result stays the same, whatever changes in the CLI or the spec.

The expected values were derived by hand from draft 0.2 of the spec. `tests/acceptance_test.go` runs them: every file on its own, every change, and for each patch the check that it names exactly the listed paths.

## Test coverage

`sh scripts/coverage.sh` measures the share of statements in `cmd/` and `internal/` that the tests execute and fails below 99 %. CI runs it on every push and pull request and writes the report to the summary of the run. As of 2026-11-24 the value is 99.5 %.

- New code needs tests in the same change; otherwise the check fails.
- The few uncovered statements are listed by the script. They are `main` (it only calls `os.Exit`) and defensive branches that cannot be reached from outside. Do not add such branches lightly: each one eats into the 1 % margin.
- Do not lower the minimum to get a change through. Raise it when the value allows.

## Implementing the CLI

As of 2026-11-24. Steps 1 to 6 of the order below are done; v0.2.0 (2026-11-26) was the first release built by the workflows. Open: distribution (7), partial configs and the checks against the file tree from section 4 of the spec.

Go module: `github.com/jdde/riskmap`. Tests run with `go test ./...`.

### Decided

- **Language: Go.** Distribution was decisive: one statically linked binary without a runtime, built for all platforms from one CI job.
- **Built and published in GitHub Actions.** The binaries are attached to GitHub Releases.
- **Where it runs:** locally, in CI and in pre-commit hooks.

Dart was the alternative and was compared with Go in a throwaway experiment (Go 1.27.4, Dart 3.8.2, the same checks). Both passed all checks. In favour of Go:

| Point | Go | Dart |
|---|---|---|
| Targets from one machine | macOS, Linux, Windows, each x64 and arm64 | Linux only, otherwise one runner per operating system |
| Linux binary | static, also runs in Alpine | needs glibc, separate builds for Alpine |
| Detecting anchors, aliases, tags in YAML | public interface | only an internal interface of `package:yaml` |
| Size, whole run | about 3.3 MB, 12 ms | about 6.5 MB, 20 ms |

The maintainer has not worked with Go before (background: JS/TS, PHP, Dart). Keep the code plain and comment Go peculiarities ("Go note: …").

### Technical decisions from the experiment

- **Own glob matcher on bytes**, no library. The spec is the truth about the dialect, and it compares paths as UTF-8 byte sequences.
- **YAML with `go.yaml.in/yaml/v3` through the node tree** (`yaml.Node`), not by decoding into structs. Only the node tree shows anchors, aliases, tags and the position of every value.
- **The library does not check four things by itself**, so they are in our code: anchors, aliases, explicit tags (`TaggedStyle`) and duplicate keys.
- **Numbers are evaluated by our code according to YAML 1.2.** The library reads `012` as 10 and `1_000` as 1000. In YAML 1.2, `012` is the number 12 and `1_000` is text.
- **Git as a subprocess with `-z`**, no git library. Output is read as bytes.
- **`CGO_ENABLED=0`**, so the binaries stay static.
- **Few dependencies.** So far only the YAML library.

### Layout

```
go.mod
cmd/riskmap/        entry point of the CLI: arguments, exit codes, output
internal/glob/      glob dialect from section 3 of the spec
internal/config/    read and validate the rule file (sections 2 and 4); yaml.go wraps everything about YAML
internal/eval/      evaluation, merging, special cases, self-protection (sections 5 and 7)
internal/result/    result format as JSON (section 6)
internal/input/     sets of paths from a patch and from git (section 10.2)
tests/              example projects, acceptance cases and the Go test for them
```

Dependencies run in one direction: `cmd` → `result`, `input`, `eval` → `config` → `glob`.

### Commands

`riskmap help` shows the full description.

| Command | Purpose |
|---|---|
| `riskmap classify SOURCE` | Classify a change and print the JSON result of section 6. |
| `riskmap check [--fail-on LEVEL] SOURCE` | Same input and same JSON as `classify`, plus a `check` object with the verdict, and exit code 3 if the risk reaches `LEVEL` (default `high`). An incomplete input always fails. |
| `riskmap validate` | Check the rule file and run its `tests` block (spec §4 and §8). That is why there is no separate `riskmap test`. Its JSON has `rules`, `tests` and the status `tests-failed`. |
| `riskmap backtest` | later, see feature ideas |

Sources for `classify` and `check`, mutually exclusive:

| Source | Rule file |
|---|---|
| `--base REF [--head REF]` | from the merge base (spec §10.1); `policy.ref` names it |
| `--staged` | from `HEAD`; for pre-commit hooks |
| `--patch FILE` | from the working directory |
| `PATH...` or NUL-separated paths on stdin | from the working directory |

`--root DIR` names the directory of the rule file, `--incomplete` marks the set of paths as incomplete. Options come before the paths; an option after a path is a usage error, because Go's flag package would otherwise read it as a path. `--` ends the options.

### Output for other programs

The output is the basis for other people's tooling, so its shape is a contract (decided 2026-11-24):

- **JSON is the default of every command.** `--format text` prints the report for people.
- **Standard output holds exactly one JSON document** for the exit codes 0, 1 and 3, and nothing for exit code 2. Messages for people go to standard error, in both formats.
- **`schema/result.schema.json` and `schema/validate.schema.json` describe the documents.** `cmd/riskmap/machine_test.go` checks the output of every kind of result against them and fails on a field the schema does not name. When you add or change a field, change the schema, the README section "Building on the output" and the tests in the same change.
- **Fields are only added, never renamed or removed,** while `"riskmap": 1` stays.

Exit codes: `0` a result with `ok` or `incomplete` (classify), below the limit (check), a valid rule file (validate); `1` rule file missing or invalid, or a test fails (`classify` still prints JSON with `no-config` or `invalid-config`); `2` wrong usage, unreadable input or a git error, then without JSON; `3` `check` reached `--fail-on`.

`check` is a convenience on top of the format, not part of the spec: the spec leaves gates to tools, and `check` is the smallest such tool. The same goes for its `check` object in the JSON and for the whole output of `validate`.

### Order

1. Glob matcher with the cases from section 12 of the spec as table tests. **Done.**
2. Read and validate the rule file, with error messages including file and position. **Done.**
3. Evaluation and result format. **Done.**
4. Inputs: paths from stdin, patch file, git refs. **Done.**
5. Acceptance run as a Go test that reads `tests/acceptance/*.yml` and runs in CI. **Done.**
6. Release workflow in GitHub Actions: six targets, checksums. **Done**, except provenance: the binaries carry no attestation yet. The workflow sets the version that `riskmap version` prints; a binary built any other way prints `dev`. The binaries of v0.2.0 still print `dev` and are stamped as built from a modified tree, because that build created its archive folders inside the checkout.
7. Distribution beyond that: install script, GitHub Action, pre-commit hook, npm package. The README points to the archives of the GitHub release and to `go install`; update its "Install" section when this lands.

After that or in between:

- **Partial configs (section 9).** Open question 1 of the spec can still change them. `config.Parse` only reads the root file so far; a `.riskmap.yml` in a subdirectory is not read, but as a path it is covered by self-protection.
- **Checks against the file tree (section 4):** globs without a match, `except` without effect, covered rules. `validate` only checks the file itself and its tests so far.
- **Conformance suite (section 12)** as separate, language-neutral cases. The Go tests cover the areas but are tied to this implementation.

### Still to clarify

- **May `default` be lower than the strictest level?** The spec allows it, and so does the CLI.
- **Windows:** the binaries build, but in CI the tests only run on Linux and macOS.
- **Not checked yet:** whether the pre-commit framework builds the Go hook without further ado.

## Core decisions of the spec

Anyone who changes the spec should know these points. The reasons are in section F of the inventory.

- **Merging instead of first match:** all matching rules count, per dimension the strictest value wins. The order of the rules has no meaning.
- **Monotonicity:** an additional rule can only make things stricter. Self-protection and partial configs rely on this.
- **Fail closed:** unmatched paths get `default`. Incomplete input gives the strictest result. A missing or invalid rule file gives no result, and tools must treat that like the strictest level.
- **Self-protection:** every `.riskmap.yml` is fixed at the strictest level. A change is judged by the rules of the base, never of the head.
- **Typed dimensions:** `risk` (`low`, `medium`, `high`) is predefined. Further dimensions are `ordered`, `set` or `flag`. The name `review` is reserved for a follow-up version.
- **Small glob dialect:** `*` stays in the segment, `**` only as a whole segment, patterns are anchored, no negation, exceptions via `except`.
- **The trace is mandatory:** the JSON result names rule, glob and `reason` and is deterministic.

## Known gaps and inconsistencies

Found on 2026-11-21 when comparing the spec with the research. Not corrected yet.

- **Reliability of the research:** blocks A and B were read in the code. A14 and B31 were not accessible. B14, B35, B37 and B40 rest on summaries of issue pages. Blocks C, D and E of the inventory come from web search hits and are unverified. "53 surveyed implementations" in the spec and the README is therefore generous.
- **Spec §7:** "only four" implementations with hard-wired self-protection is the state of round 1. It is also fixed in the code of B1, B34, B39 and A13.
- **Spec §5.1 and inventory G:** B32 is counted under "an empty list gives the lowest level". In B32 the empty list gives A, and A is the strictest level there.
- **Default:** the README recommends a high default. The spec only requires that `default` contains a `risk`; `default: { risk: low }` would be valid.
- **Inventory A:** the header line mentions A10, but there is neither a row nor a profile for it.
- **Inventory E:** the feature catalogue is still in the state before the originals were read.
- **Spec §10.2, patch as input:** the spec requires NUL-separated paths and forbids git's quoted form. When a tool reads a patch, quoted paths are unavoidable. The case is not covered. The CLI decodes the quoting (`internal/input/patch.go`) and aborts without a result on a patch it cannot read safely.

Places the draft leaves open and the CLI has decided provisionally. Each belongs in the spec once confirmed:

- **§6, labels without a value:** in the example of the spec the overall result carries `"protected": false`, while the entries under `paths` leave out dimensions without a value. The CLI writes the same shape everywhere: `set` always (empty as `[]`), `flag` always (`false`), `ordered` only with a value.
- **§6, trace without a path:** `empty-change` and `incomplete-input` belong to no path. The CLI writes them into a top-level `because`, which the spec does not know.
- **§6, order in the trace:** rules sorted by ID, then `default`, then `builtin`. A rule without an `id` is named by its list index from 0; swapping such rules changes the trace.
- **§6, `policy.ref`:** missing when the rule file comes from the working directory.
- **§6, `dimensions`:** the result repeats the declared dimensions with their type and, for `ordered`, the values from mildest to strictest. The spec does not know the field. Without it a tool cannot compare levels unless it reads the rule file from the merge base itself.
- **§6, `path_base64`:** a path that is not valid UTF-8 additionally carries its exact bytes in base64, because two different such paths can look the same after the replacement with U+FFFD.
- **§6, path that cannot be represented:** invalid UTF-8 cannot be written as a JSON string. The output replaces the bytes with U+FFFD. An empty path also counts as `unreadable-path`.
- **§2, types of values:** judged by the core schema of YAML 1.2. Where text is expected, an unquoted `123`, `true` or `null` is an error with a hint to quote it. `yes` and `on` are text.
- **§2.1, `x-*`:** allowed at the top level, in a rule, in a dimension and in a test. A dimension name must not start with `x-`.
- **§2.1, duplicate glob:** counts across `match` and `except` of one rule together.
- **§2.1, minor points:** `rules: []` and an empty `except` are valid. Duplicate values in a `set` are silently merged. `values` on a `set` or `flag` is an error.
- **§7, self-protection for `set`:** the values from `default` are added even when a rule matches the rule file and the default would otherwise drop out.
- **§2.2, strictest value of a `set`:** all values from `default` and the rules, not those from `tests`.
- **§10.2, renames:** `--base` and `--staged` switch off git's rename detection, so the old and the new path both appear. In a patch, the source of a copy (`copy from`) does not count.
- **§3, spelling of a path:** a path with a segment `.`, `..` or an empty segment (which covers a leading or trailing `/`) is not matched and gets the strictest result with the reason `unreadable-path`. Otherwise `./secrets/key` would slip past a rule for `secrets/**`. Git never writes such paths; they only come in as arguments or on standard input. On Windows, `\` in a path argument is turned into `/`.
- **§3, three places in the glob dialect**, decided provisionally by `internal/glob`:
  - `?` matches one character in the sense of a Unicode code point, not a byte. `?ber` matches `Über`.
  - `!`, `{`, `}`, `[`, `]` and backslash are an error anywhere in a pattern, not only at the start. Files with these characters in their name can only be matched with `*` or `?`.
  - A segment `.` is an error, like `..`. A trailing `/` is an error as well, being an empty segment.

The eight open questions of the spec are in its section 13.

## Feature ideas

Ideas that are not in the spec yet. Append new ideas here, with date and occasion.

### Backtest against old pull requests

Added 2026-11-21. Related to open question 8 of the spec.

**Idea:** a command evaluates a `.riskmap.yml` against the last N pull requests of a repository. An agent can run it while configuring and sees how the current configuration would have acted on these PRs before it is switched on.

**Occasion:** the most common reason why risk levels were rolled back or rejected in the surveyed projects was a badly calibrated list:

- B36 shortened its list because directory patterns made almost every PR risky.
- In B8, 52.7 % of new PRs were `risk:high`.
- B14 rejected levels because only about 7 % of the last 40 PRs would have been routed differently.
- B30 uses old PRs with a known target level as a regression test, B5 has a backtest script for its thresholds.

With `default: high` this hits Riskmap in particular: every path without a rule is high.

**Sketch** (a proposal, nothing is decided):

- Input: a rule file and a set of old PRs, each as a list of paths. Old and new paths of renames and deleted files count as in section 10.2 of the spec.
- The backtest judges all PRs with the rule file under test. That deliberately deviates from section 10.1, which requires the rules of the base in operation.
- Output as JSON, so an agent can evaluate it:
  - Distribution of the PRs per value of each dimension, for example the share of `risk: high`.
  - Share of PRs whose result comes only from `default`. That shows missing rules.
  - Per rule: how many PRs it matched and how often it determined the result. Rules without matches and rules that match almost everything stand out this way.
  - The PRs per level with their trace, as a sample to read.
- Comparison of two rule files: which PRs would be classified differently. That makes a rule change reviewable and answers B14's question of how many PRs would be routed differently at all.
- Read-only. The backtest sets no labels and no status.

**Open:**

- Does the command belong to the reference tool or is it a separate tool? The CLI plan lists it as `riskmap backtest` for now.
- Where do the path lists come from: the local git history of merge commits or the API of the host? The local variant needs no token but sees squash merges only as one commit.
- Should the spec standardise the output format of the backtest, or is the result format of section 6 per PR enough?
- Are there warning thresholds, such as "more than X % of PRs are `high`", or does the judgement stay with the agent?
