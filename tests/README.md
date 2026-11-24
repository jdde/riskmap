# Tests

This folder contains example projects with a `.riskmap.yml` and the results a Riskmap CLI must produce for them.

It serves two purposes:

- **Illustration:** The projects show what a Riskmap can look like for a typical Next.js, Laravel or Flutter project.
- **Acceptance test:** The patches and expected results are the baseline the CLI runs against. As long as the rule file of an example does not change, the classification must stay the same, no matter what changes in the CLI or in the specification.

The expected results are derived by hand from draft 0.2 of the specification (`SPEC.md` in the repo root). `acceptance_test.go` in this folder runs them, together with all other tests:

```
go test ./...
```

For each example the test checks three things: the classification of every single file (`files`), the classification of every change (`changes`), and that every patch names exactly the paths its case lists under `paths`. In addition, `files` must contain every file of the example project.

To reproduce a case by hand:

```
cd tests/examples/nextjs-web
riskmap classify --patch ../../acceptance/nextjs-web/checkout-page.patch
```

## Layout

```
tests/
  examples/
    nextjs-web/        Next.js (App Router), Prisma, NextAuth, Stripe
    laravel-api/       Laravel API with Sanctum
    flutter-app/       Flutter app with an Android and an iOS part
  acceptance/
    <example>.yml      expected results for one example
    <example>/         one .patch file per case
  acceptance_test.go   Go test that runs the cases
  make-patches.py      script that regenerates the .patch files
```

The example projects are skeletons. They follow the structure their framework prescribes, but they do not run: no dependencies are installed, and the lockfiles are placeholders.

Each example has its `.riskmap.yml` in its own root directory. The CLI must run with the example folder as the repo root, not with the root of this repo.

| Example | Dimensions besides `risk` | Mainly shows |
|---|---|---|
| `nextjs-web` | `domain` (set), `protected` (flag) | `except`, one file in two rules, special characters in the path (`(shop)`, `[...nextauth]`) |
| `laravel-api` | `domain` (set), `irreversible` (flag), `data` (ordered) | a broad lenient rule plus a narrow strict rule, a custom ordered dimension |
| `flutter-app` | `domain` (set), `platform` (set) | two set dimensions, `except` for translations, a wildcard in the file name |

## Expected results

Each file `acceptance/<example>.yml` has two parts.

### `files`

For every file of the example project, the expected labels when only this one file is changed. The list is complete: when a file is added to the project, it must be entered here.

```yaml
files:
  "src/components/Button.tsx": { risk: medium, domain: [ui], protected: false }
```

### `changes`

Changes made of several files, and the special cases of the specification.

```yaml
changes:
  - id: component-with-test
    description: "Component and its test: the higher value wins"
    patch: "nextjs-web/component-with-test.patch"
    paths: ["src/components/Button.tsx", "src/components/Button.test.tsx"]
    expect:
      status: ok
      labels: { risk: medium, domain: [tests, ui], protected: false }
```

| Field | Meaning |
|---|---|
| `id` | Name of the case, unique per file |
| `patch` | Patch file relative to `tests/acceptance/`. Missing for cases that cannot be expressed as a patch. |
| `paths` | The path set of the change. For cases with a patch, this is the set the CLI must read from the patch. |
| `complete` | `false` marks an incomplete path set. The default is `true`. |
| `expect.status` | Expected `status` of the result |
| `expect.labels` | Expected labels for the whole change |

Two cases per example have no patch: `empty-change` (empty path set) and `incomplete-input` (incomplete path set).

### Comparison

Expected and actual results are compared by this rule:

- A `set` dimension without a value counts as `[]`. The order of the values does not matter.
- A `flag` dimension without a value counts as `false`.
- An `ordered` dimension that is missing from the expectation must not have a value in the result.
- After that, all dimensions must match exactly.

The trace (`because`) and the fields under `policy` are not compared. They change with every rule ID and every hash; the classification does not.

The `tests` block in the `.riskmap.yml` files, in contrast, follows section 8 of the specification: dimensions that are named must match, dimensions that are not named are not checked.

## The patches

The patches are generated with `git diff --cached -M --binary` against the respective example project and can be checked there with `git apply --check`. The paths are relative to the example folder.

`make-patches.py` generates them. It copies each example project into a temporary git repository, applies the edits of each case, and writes the diff to `acceptance/<example>/<case>.patch`. It also sets the `patch:` line of each case in `acceptance/<example>.yml`. Run it with `python3 tests/make-patches.py`; it needs only Python 3 and git.

From a patch, the CLI must read the path set and evaluate it with the `.riskmap.yml` of the example as it lies in the folder. This matches the rule from section 10.1 of the specification: a change is evaluated with the rules that were in force before it.

Some patches specifically test how paths are read:

| Patch | What it tests |
|---|---|
| `nextjs-web/rename-out-of-auth.patch` and the other `rename-*` | Pure rename without `---`/`+++` lines. Both the old and the new path count. |
| `nextjs-web/docs-with-special-characters.patch` | Path with a space and non-ASCII characters. Git writes it quoted (`"a/docs/\303\234ber uns.md"`). |
| `flutter-app/assets-and-docs.patch` | Binary file (`GIT binary patch`) |
| `laravel-api/deleted-test.patch` | A deleted file counts through its path |
| `*/near-miss.patch` | New files whose paths only resemble a rule (`docs2/`, `tests2/`, `library/`) |
| `*/riskmap-config.patch` | The patch lowers `default` to `low`. The result stays `high`: the rule file protects itself, and the change is evaluated with the state before the patch. |

## Maintenance

- **Rule file of an example changed:** The expected results may only change if a different classification is actually intended. Rewording the rules without that intent must leave all cases passing unchanged.
- **File in the example project changed:** Patches that touch this file may no longer apply afterwards. `git apply --check` in the example folder shows that. Running `make-patches.py` again regenerates all patches from the current example projects; if an edit in the script no longer finds its text, the script stops with an error.
- **CLI or specification changed:** The expected results stay as they are. If a spec decision changes a result, that is a deliberate change to the fixtures and belongs in the same commit.

## Scope

This folder is not the conformance suite from section 12 of the specification. The suite tests the glob dialect, invalid rule files and partial configs with purpose-built minimal cases. The examples here test whether realistic configurations reliably produce the same result.
