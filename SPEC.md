# `.riskmap.yml` – Riskmap specification, v1 (Draft 0.2)

Dated 2026-11-17. Based on a survey of 53 existing implementations and proposals, listed in [`docs/prior-art.md`](docs/prior-art.md). References such as "A4" or "B13" point to the implementation a decision comes from.

The words MUST, MUST NOT, SHOULD and MAY are normative.

## 1. Purpose and scope

The format assigns labels to the files of a repository by pattern. It decides nothing: what follows from the labels (briefing, required review, merge gate) is up to the tool that reads the result.

The specification has three parts:

| Part | Content | Sections |
|---|---|---|
| Format | rule file, glob dialect | 2–4 |
| Evaluation | algorithm, error cases, result format | 5–8 |
| Requirements for tools | policy source, path input, approvals | 10 |

Not part of v1: rules on file content, downgrading by proof, scoring, LLM signals. Section 11 describes how they attach.

Also not part of v1 is who is responsible for a review (who reviews, dev or product). That is settled outside the format, for example in the ticket. The format only says how sensitive a change is. Review responsibility is planned as the first feature of a follow-up version (section 14).

## 2. Rule file

The file is named `.riskmap.yml` and lives in the repository root. Further files of the same name MAY live in subdirectories (section 9). The format is YAML 1.2 without anchors, aliases and tags.

```yaml
version: 1

dimensions:                      # optional; risk is predefined
  domain:    { type: set }
  protected: { type: flag }

default: { risk: high }

rules:
  - id: specs
    match: ["**/*.feature", "specs/**/*.md"]
    labels: { risk: low, domain: [credits] }
    reason: "Business behaviour, no effect at runtime"

  - id: webhooks-in
    match: ["apps/*/src/webhooks/incoming/**"]
    except: ["apps/*/src/webhooks/incoming/**/*.md"]
    labels: { risk: high }
    reason: "External entry points, core flow"

  - id: ci
    match: [".github/workflows/**"]
    labels: { risk: high, protected: true }
    reason: "Rules must not downgrade themselves"

tests:
  - path: "specs/credits/topup.feature"
    expect: { risk: low, domain: [credits] }
  - path: "apps/api/src/webhooks/incoming/stripe.ts"
    expect: { risk: high }
```

### 2.1 Fields

| Field | Required | Meaning |
|---|---|---|
| `version` | yes | Integer. This specification defines `1`. An unknown value is an error. |
| `dimensions` | no | Declares labels beyond `risk`. Root file only. |
| `default` | yes (root) | Labels for paths that no rule matches. MUST contain `risk`. Root file only. |
| `rules` | yes | List of rules. Their order has no meaning. |
| `rules[].id` | no | Stable name for the trace and for tests. Unique per file. If missing, the list index is used. |
| `rules[].match` | yes | Non-empty list of globs. |
| `rules[].except` | no | List of globs. A path that matches here does not match the rule. |
| `rules[].labels` | yes | At least one label. |
| `rules[].reason` | yes | Non-empty text. Appears in the result. |
| `tests` | no | Expected results for example paths (section 8). |
| `x-*` | no | Keys with the prefix `x-` are reserved for extensions and are ignored by the core evaluation. |

Any other unknown key, a duplicate key, a duplicate `id` and a glob named twice in the same rule are errors (A4, A6, B3).

### 2.2 Dimensions

Every label belongs to a dimension with a type. The type determines how several values are merged.

| Type | Value | Merging | Strictest value |
|---|---|---|---|
| `ordered` | one of `values` (ascending) | maximum | last entry of `values` |
| `set` | list of strings | union | all values that occur in the rule file |
| `flag` | `true` / `false` | logical or | `true` |

Predefined is:

```yaml
risk: { type: ordered, values: [low, medium, high] }
```

`risk` MUST NOT be redefined. The name `review` is reserved for a follow-up version (section 14) and MUST NOT be declared as a dimension in v1. A label whose dimension is not declared is an error.

All three ways of merging are monotonic: an additional rule can only make a result stricter, never milder. Sections 7 and 9 rely on this. Models: B38 (the strictest per property), B13 (union over matches).

## 3. Glob dialect

The dialect is small on purpose. This is where the surveyed implementations differ most (section F of the inventory).

| Rule | Definition |
|---|---|
| Reference | Path relative to the directory of the rule file, with `/` as separator, without a leading `/` or `./` |
| Anchoring | A pattern MUST match the whole path. A pattern without `/` only matches files directly in the reference directory. |
| `*` | zero or more characters except `/` |
| `?` | exactly one character except `/` |
| `**` | only as a whole segment; matches zero or more directories. `a/**` matches everything below `a/`, `**/x` matches `x` at any depth including the root. |
| Dotfiles | A leading dot is an ordinary character. `*` and `**` also match `.env` and `.github/`. |
| Case | is significant |
| Not allowed | Negation (`!`), brackets (`{}`, `[]`), backslash, `..`, empty segments, a leading `/`. Each of these is an error. |
| Compared | the byte sequence of the path as UTF-8, without Unicode normalisation |

Exceptions are expressed with `except`, not with negation (A4, B11).

## 4. Validation

A rule file is invalid if one of the rules from 2.1, 2.2 or 3 is violated. An evaluator MUST reject an invalid file and MUST NOT fall back to defaults.

A validator SHOULD additionally check against a file tree:

| Check | Level | Model |
|---|---|---|
| Every glob in `match` matches at least one existing file | warning; an error in strict mode | B13, B19, B33, B36 |
| Every `except` glob takes at least one file out of its rule | warning | – |
| All `tests` pass | error | B22 |
| Rules whose `match` is completely covered by a stricter rule | note | B19 |

## 5. Evaluation

The input is a set of paths and a completeness flag. The output is a result as described in section 6.

For each path:

1. Find all rules of all rule files in effect (section 9) where a `match` glob matches and no `except` glob matches.
2. Add the built-in rules that match (section 7).
3. If no rule from step 1 matches, the labels from `default` apply. Built-in rules alone do not prevent the default.
4. Merge the labels of all sources in effect, per dimension (2.2).

The result for the whole change is the merge of the results of all paths.

There is no first match. The order of the rules, of the files and of the paths MUST NOT affect the result. Eight surveyed implementations use first match; there an early broad rule beats a later strict one (B2, B4, B15, B18).

### 5.1 Special cases

Every case has a fixed result and its own reason in the trace. "Strictest result" means: in every dimension the strictest value according to 2.2.

| Case | Result | `status` | Reason |
|---|---|---|---|
| Path matches no rule | `default` | `ok` | `default` |
| Set of paths is empty | `default` | `ok` | `empty-change` |
| Set of paths is incomplete | strictest result | `incomplete` | `incomplete-input` |
| Path cannot be represented (invalid UTF-8, contains NUL) | strictest result for this path | `ok` | `unreadable-path` |
| Rule file is missing | no label result | `no-config` | `no-config` |
| Rule file is invalid | no label result | `invalid-config` | `invalid-config` |

With `no-config` and `invalid-config` there are no dimensions from which a strictest result could be derived. A tool that derives gates MUST treat both like the strictest handling it knows (section 10). An empty set of paths never gives a lower level than `default`; twelve surveyed implementations classify it as the lowest level.

## 6. Result format

The result is JSON. The trace is mandatory: it is the basis for briefings and audits.

```json
{
  "riskmap": 1,
  "status": "ok",
  "policy": { "ref": "4f2a…", "files": [{ "path": ".riskmap.yml", "sha256": "9c1e…" }] },
  "labels": { "risk": "high", "domain": ["credits"], "protected": false },
  "paths": [
    {
      "path": "specs/credits/topup.feature",
      "labels": { "risk": "low", "domain": ["credits"] },
      "because": [
        { "kind": "rule", "config": ".riskmap.yml", "rule": "specs", "glob": "**/*.feature", "reason": "Business behaviour, no effect at runtime" }
      ]
    },
    {
      "path": "apps/api/src/webhooks/incoming/stripe.ts",
      "labels": { "risk": "high" },
      "because": [
        { "kind": "rule", "config": ".riskmap.yml", "rule": "webhooks-in", "glob": "apps/*/src/webhooks/incoming/**", "reason": "External entry points, core flow" }
      ]
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `status` | `ok`, `incomplete`, `no-config`, `invalid-config` |
| `policy` | ref and hash of every rule file that was read (B39) |
| `labels` | result for the whole change |
| `paths[].because[].kind` | `rule`, `default`, `builtin`, `empty-change`, `incomplete-input`, `unreadable-path` |
| `errors` | with `invalid-config`: list with file, position and message |

`set` values are written sorted. The same input MUST give byte-identical output.

## 7. Self-protection

Protecting the rule file is part of the format and not a rule someone can forget. Only four of the surveyed implementations hard-wire it (A2, A6, B2, B3).

Built-in rule, always active, cannot be switched off:

| Path | Effect |
|---|---|
| `.riskmap.yml` in any directory | strictest value in every `ordered` and `flag` dimension; `set` dimensions get the values from `default` |

The reason in the trace is `builtin` with `rule: "riskmap-config"`. A repository protects further paths (workflows, CODEOWNERS, agent instructions) with its own rules; tools MAY bring additional built-in rules and MUST mark them as `builtin` in the trace.

The second part of self-protection is the policy source (section 10.1).

## 8. Tests in the rule file

`tests` records example paths with their expected result. `expect` is a subset: dimensions it names must match exactly, dimensions it does not name are not checked.

```yaml
tests:
  - path: "docs2/readme.md"        # near miss: must not count as docs/
    expect: { risk: high }
```

Tests run against single paths, without a file tree. They make rule changes reviewable and serve as examples for readers (B22, B16).

## 9. Partial configs in a monorepo

A `.riskmap.yml` in a subdirectory applies to all paths below it.

| Rule | Definition |
|---|---|
| Reference | Globs are relative to the directory of the partial config. |
| Allowed fields | `version`, `rules`, `tests`, `x-*`. `default` and `dimensions` are errors there. |
| Effect | Its rules are added to those of the root file and of all configs on the way there. |
| Default | The root default only drops out for a path when some rule of some config in effect matches. |

Because all merging is monotonic, a partial config can only make things stricter. It can neither cancel a root rule nor lower a label. That needs no check of its own; it follows from section 5 (compare B3, B15).

One peculiarity: if a partial config matches a path with a mild rule, the strict root default drops out for that path. This is intended (a project classifies its own files), but it is the only place where a partial config can lower a result. See open question 1.

## 10. Requirements for tools

A tool conforms if it implements the evaluation of section 5, writes the result of section 6 and keeps to the following points.

### 10.1 Policy source

- The rule files MUST be read from the merge base or the base SHA of the change, never from the head. A change is thus judged by the rules that were in force before it (A6, B3, B4, B9, B28, B32, B33).
- The code of the evaluator MUST NOT come from the head of the change.
- For stacked PRs the default branch MAY be used instead (B2). The result names the ref that was used in `policy.ref`.

### 10.2 Path input

- For renames both the old and the new path count. Deleted files count (A1, A6, A7, A9).
- Paths are read NUL-separated, never in git's quoted form (B4, A13).
- If the tool cannot show that the list is complete (for example an API limit of 3000 files, or a mismatch in counts), it MUST mark the input as incomplete.

### 10.3 Derived decisions

- A tool that derives gates MUST treat `incomplete`, `no-config`, `invalid-config` and a crash of its own like the strictest handling. A missing result is never an approval.
- Auto-merge SHOULD be switched off before the evaluation and only be re-enabled after a result (B28, B6).
- The result SHOULD be recomputed at merge time instead of trusting a label that was set earlier (B11, B18).
- Within one change, a result that was reached once SHOULD NOT drop automatically (B3, B35).

### 10.4 Approvals

- An approval MUST be bound to the head SHA it was given for (B5, A4, A2).
- It MAY survive a merge of the base branch if that merge is free of conflicts and the machine-made merge result corresponds to the commit (B32, B10).
- Approvals by the author and by bots do not count as human approval (A2, B40).
- After an approval the corresponding check MUST be evaluated again (B26).

### 10.5 Override

The classification follows only from the changed paths. Author, PR text and commit messages MUST NOT affect it. A human override is not part of the format. A tool that offers one MUST record who set it when and for which head SHA, and MUST NOT allow the author to set it.

## 11. Extensions outside the core

These features occurred in the surveyed implementations and are not standardised in v1. They attach through `x-*` keys and additional `because` entries.

| Extension | Rule for tools | Model |
|---|---|---|
| Rules on changed lines (added, removed) | May only make stricter. Content that cannot be read counts as a match. | A4, A6, B8, B24 |
| Size, number of affected projects | May only make stricter. | A6, B30 |
| LLM assessment | May only make stricter. An error of the model counts as the strictest value. | A13 |
| Downgrading by proof (pure revert, change only in test blocks) | Only with machine-checkable evidence that is part of the result. | B12, B8 |
| Generating CODEOWNERS or `labeler.yml` | Generate from the rule file instead of maintaining in parallel. | A4, C1, B26 |
| Report-only mode | Compute the result and report deviations without blocking. | B8, B25 |

## 12. Conformance tests

A test suite belongs to the specification. Minimum scope, each case with rule file, set of paths and expected JSON:

| Area | Cases |
|---|---|
| Glob | `*` does not cross `/`; `**` matches zero directories; a pattern without `/` only in the root; dotfiles; case; near misses (`appx/`, `docs2/`) |
| Invalid globs | `!`, `{}`, `[]`, backslash, `..`, leading `/` |
| Merging | a broad mild rule before and after a narrow strict rule gives the same result; `set` forms the union; `flag` is or-ed |
| `except` | an exception only applies to its own rule |
| Special cases | all six rows from 5.1 |
| Self-protection | change to the root config and to a partial config; a mild rule on `.riskmap.yml` does not lower |
| Partial configs | relative globs; `default` in a partial config is an error; a partial config makes stricter |
| Paths | space, quotation mark, line break, non-ASCII, U+2028 in the file name |
| Schema | unknown key, duplicate `id`, unknown dimension, declaration of `review`, value outside `values`, missing `reason` |
| Determinism | swapping the order of rules and paths gives byte-identical output |

## 13. Open questions

1. **Default and partial configs:** Should a partial config be allowed to replace the root default for its paths (this draft), or should the default be a floor that only the root file can go below? The second is stricter but forces every mild rule into the root file.
2. **Risk levels:** `risk` is fixed to `low`/`medium`/`high` so that tools share the same meaning. Some implementations have a fourth level for irreversible things (A4 `one-way-door`, A2 `CRITICAL`). Model it as a separate `flag` dimension or extend `risk`?
3. **Review responsibility:** removed from v1 (decision of 2026-11-17), planned as the first feature of the follow-up version. See section 14.
4. **Expectation per level:** Should a dimension be able to describe what a person does per value (read the whole diff, read a summary)? Models: B21, A6 `guide`.
5. **Organisation-wide floors:** Is an `extends` to a central rule file needed (B3, A7), or is it enough that tools bring additional built-in rules?
6. **Case:** The draft treats case as significant. On macOS and Windows a path in different case can mean the same file (B30 therefore lowercases before matching). Is a lint enough, or should the format become case-insensitive?
7. **Relation to Nx:** Should rules be able to match Nx projects or tags besides globs (C1), or does that stay an extension that resolves tags into globs before the evaluation?
8. **Backtest:** Before adoption, run the rules against the last 100 PRs and measure how many would be routed differently (B14, B30). Does a backtest command belong to the reference tool?

## 14. Outlook on follow-up versions

The order is a plan, not a promise. Every follow-up version stays backwards compatible: a valid v1 file stays valid and gives the same result.

### 14.1 First feature: review responsibility

In v1 Riskmap only says how sensitive a change is. Who reviews it comes from the ticket. The first feature planned after v1 is a predefined dimension for responsibility:

```yaml
review: { type: set }        # predefined from the follow-up version on

rules:
  - id: specs
    match: ["**/*.feature", "specs/**/*.md"]
    labels: { risk: low, review: [product] }
    reason: "Business behaviour, validated by the product owner"
```

v1 is prepared for this:

- The type `set` with union over all matches already exists (2.2).
- The name `review` is reserved in v1: a v1 file MUST NOT declare a dimension of its own with this name. So no repository collides with the later meaning.
- Whoever wants to try out responsibility in advance declares their own `set` dimension under another name, for example `audience`.

Open for the follow-up version:

| Question | Models |
|---|---|
| What is an audience: just a name, or team and people in the format? | B32 (logins per role), A9 (teams) |
| Is `all-of`/`any-of` needed, or is the union enough? | A9 |
| How does responsibility from the Riskmap relate to that from the ticket: floor, addition or suggestion? | B12, A2 (path as a floor under a declaration) |
| Does responsibility depend on the level (required only from `high`)? | B13 |

### 14.2 Further candidates

In the order the surveyed implementations suggest: rules on changed lines (11), organisation-wide floors via `extends` (open question 5), a description of the expected action per level (open question 4), matching by Nx projects and tags (open question 7).
