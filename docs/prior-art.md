# Prior art: path-based risk and review classification

As of: 2026-11-14. Working basis for the open standard (`.riskmap.yml`).

**Important note on reliability:** Block A and block B were read in the original on 2026-11-16/17: code, config, tests and docs of the cloned default branches; nothing was executed. Blocks C, D and E still come from web search hits and their summaries and need to be checked against the original when going through them. Of the prior assumptions that were checked, about half were wrong in at least one point. `?` means: not discernible from the search hits.

Columns: **Config** = rule file, **Tiers** = value range, **Default** = tier for unmatched paths, **Notable** = what is worth looking at for the standard.

## A. Reusable tools and actions

**Status: read in the original (2026-11-16), except A10 and A14.** The per-project profiles with details and `path:line` evidence are not part of this repository; they are in the git history (commit bb5d6a8).

| # | Project | Config | Tiers | Combination per file | Default (unmatched) | Policy source | Notable | Weaknesses |
|---|---|---|---|---|---|---|---|---|
| A1 | [0xgui/agent-pr-gate](https://github.com/0xgui/agent-pr-gate) | `.review-gate.yml`, minimatch (`dot: true`) | low / medium / high | Maximum | `default_tier`, default medium | Base commit (switchable via `config-ref: head`) | Renames with both paths; truncated list = high + block; trace with glob and config line; JSON with `schema_version`; negation and globs that can never match are config errors | Unknown keys ignored; missing config = defaults; no self-protection of the rule file; no version field |
| A2 | [lightninglabs/review-policy](https://github.com/lightninglabs/review-policy) | `.github/review-policy.yml`, own glob matcher, `version: 1` mandatory | LOW … CRITICAL | Path is only a floor, raises | none: without a severity label FAIL | `pr.base.ref` (branch tip) | Tier comes from a maintainer label, paths only as a floor; fail closed throughout; policy file fixed at CRITICAL; author labels/approvals do not count; `fresh`/`carry` govern head binding per tier | Implementation only in PR #3, `main` contains only a README; no trace of the triggering glob |
| A3 | [vtri950/risk-gate](https://github.com/vtri950/risk-gate) | YAML, mixed fnmatch/regex | P0 / P1 / P2 | First match | P2 (lowest) | PR checkout | – | Fail open almost everywhere; deletions and old rename paths invisible; no tests; dead config field; possible injection via file names (not tried) |
| A4 | [eggdev/clutch](https://github.com/eggdev/clutch) | `.clutch/risk.yaml`, `schemaVersion: 1`, Bun.Glob | low / medium / high / one-way-door | Maximum | Mandatory field `unmatched` | Base branch (label), default branch (auto-merge) | `except` globs; `lines` (regex on changed lines); `journeys` (rule is dropped if tests exist); strict validation; CODEOWNERS is generated from `one-way-door`; owner approval bound to head SHA | Rule file does not protect itself; empty change = low; `unmatched: medium` falls within auto-merge |
| A5 | [juw12/gate](https://github.com/juw12/gate) | `gate.toml`, fnmatch (`*` crosses `/`) | 0 / 1 / 2 from lists `risk.high`/`risk.low` | High match wins | 1 | Target branch via `git show` | Local merge-queue CLI, not a PR gate; approval as commit trailer `Approved-By`/`Risk-Tier`, `gate audit` checks after the fact; attack on the policy is tested | No trace; approver is free text; missing file = defaults |
| A6 | [attrecto/merge-risk](https://github.com/attrecto/merge-risk) | `.merge-risk.yml`, zod schema, `version: 1` and `default-tier` mandatory, picomatch | safe / risky / very-risky | Maximum of zone, content pattern, protected path | `default-tier` | Base SHA | GitHub, GitLab and CLI; policy and workflow fixed at very-risky; every error = status `error`; typed reasons; `approvals` and `guide` per tier; size raises by one tier; no signal lowers | Empty list = safe; no override, no audiences, no inheritance |
| A7 | [palantir/policy-bot](https://github.com/palantir/policy-bot) | `.policy.yml`, regex (`paths`) and globs (`globs`, doublestar) | no tiers, rules with predicates | all active rules, `and`/`or` | all rules skipped = `error` | Target branch; fallback org repo `.github`, remote reference | `policy-bot validate` and offline scenario tests; result tree with a reason per predicate; simulation API; renames as deletion + creation; >3000 files = error | Invalid globs silently never match; `or` can hide errors; no self-protection found |
| A8 | [Weetbix/configurable-required-approvals](https://github.com/Weetbix/configurable-required-approvals) | Workflow input, minimatch | Approval count per group | Maximum across groups | passed | Workflow file | – | No pagination; approvals not deduplicated; "Changes requested" cancels nothing; last commit 2024 |
| A9 | [Automattic/action-required-review](https://github.com/Automattic/action-required-review) | Requirements file or input, picomatch (`dot: true`) | no tiers, teams per rule | all applicable rules | explicit `unmatched` rule, otherwise passed | Workflow checkout | Teams with `all-of`/`any-of`/`is-author-or-reviewer`; `consume` removes paths from later rules; `previous_filename` checked; fully paginated | Mirror without tests in the clone; trace only in the log |
| A11 | [jey3dayo/pr-insights-labeler](https://github.com/jey3dayo/pr-insights-labeler) | `.github/pr-labeler.yml` | risk/high, risk/medium or no label | fixed heuristic | no label | Base only under `pull_request_target`, otherwise head | Directory labeler with `include`/`exclude`/`priority` and deterministic precedence | Risk depends on CI status and commit prefix and can drop; no gate; docs deviate from the code |
| A12 | [sethdford/shipwright](https://github.com/sethdford/shipwright) | `config/policy.json` with JSON Schema | critical / high / medium / low | Cascade | low | PR checkout | Data model separates `riskTierRules` and `mergePolicy`; hardening against file names in `GITHUB_OUTPUT` | Gate enforces almost nothing; `low` patterns never evaluated; fail open throughout; classifier copied three times, without tests; docs claim more |
| A13 | [andreasasprou/open-review](https://github.com/andreasasprou/open-review) | no rule file, regex hard-coded | floor-high / model-high / normal | Floor before model | Model triage | pinned action ref | A floor match skips the model; the model may only raise; every model error = high; paths NUL-separated with special-character tests; documented bypasses of a "trivial" tier | Not configurable; only controls review depth |
| A14 | patinaproject/skills #909 | – | – | – | – | – | Not accessible (404), unsubstantiated | Remove from the evaluation |

## B. Repo-internal implementations

**Status: all read in the original (2026-11-16/17), except B31.** The per-project profiles with details and `path:line` evidence are not part of this repository; they are in the git history (commit bb5d6a8). Of 40 entries, 27 are implemented on a main branch (some only advisory; B16 classifies CI areas instead of risk), 3 exist only in PRs or feature branches (B6, B22, B27), 9 are proposals or pure documents (B14, B20, B21, B23, B24, B25, B35, B37, B40), 1 is not accessible (B31).

| # | Project | Implementation status | Config | Tiers | Combination per file | Default and error cases | Policy source | Notable |
|---|---|---|---|---|---|---|---|---|
| B1 | [TogetherWeOwn/agent-toolkit](https://github.com/TogetherWeOwn/agent-toolkit) and others (4 repos) | Map in all 4 repos; classifier in only one | `.github/review-tiers.yml` (legacy and v1 side by side) | low / ordinary / sensitive | Maximum | sensitive (also when empty, >2000 paths, missing/broken map) | PR head, self-protection hard-coded | In 3 repos no code reads the file; content-based triggers are only YAML comments; plugin parser does not understand v1, v1 maps count as sensitive across the board; git call hardened against checkout config |
| B2 | [KingInYellows/yellow-plugins](https://github.com/KingInYellows/yellow-plugins) | implemented, advisory only | `.github/review-risk.json`: `version`, `default_tier`, `low_max_lines`, `rules[]{tier, why, paths}` | low / medium / high | First match | `default_tier` (medium); broken policy high, missing medium | Default branch tip (stacked PRs) | Policy file, every `REVIEW.md` and paths with unusual characters fixed at high; JSON with `reasons` trace; `*` crosses `/` |
| B3 | [frostyard/core](https://github.com/frostyard/core) (+ frostmail, pilothouse) | implemented, review hint only | `.github/risk-paths.json`: `schema_version`, `default_level`, `paths[]{glob, level, why}`, strict | L1–L4 | Maximum, order-independent | `default_level` (L3); ≥3000 files L1; missing list: no label, job green | Base SHA via the Contents API | Hard-coded org floor; reusable workflow with SHA pin; labels as a ratchet; trace table; v1 still readable |
| B4 | Tim-van-Oudheusden ([Tim-van-Oudheusden/MinimalUI](https://github.com/Tim-van-Oudheusden/MinimalUI), Dailies, pindu-launcher) | implemented | `.github/risk-assessment.yml`: lines `- <tier> <glob>` | low / medium / high | First match (an early low rule beats a late high rule) | medium, gets auto-merged; empty = low | Base SHA | Parity test rule file ↔ `labeler.yml`; quoting bug fixed; `*` crosses `/` |
| B5 | [Osasuwu/jarvis](https://github.com/Osasuwu/jarvis) | implemented | `config/protected-paths.json`, buckets per repo | … HIGH, CRITICAL only declared | Maximum of declaration, path, size | fail closed to HIGH | Base branch tip | Approval = last admin approval with `commit_id == head`, stateless; admin authors skip the hold |
| B6 | [Aviator-Coding/home-ops](https://github.com/Aviator-Coding/home-ops) | only in PR #2165 | Python lists in the script | risk:low / normal / high | Maximum | normal (also when empty); incomplete list high | Base tip (`pull_request_target`) | Verified Renovate exception may lower (all commits from the bot and signed, blocklist); never-low list for agent control files; auto-merge is actively switched off on error; `LOW_EXCEPT` |
| B7 | [opendatahub-io/models-as-a-service](https://github.com/opendatahub-io/models-as-a-service) | implemented, advisory only | inline in the workflow, no globs | lightweight / standard | First match | standard (also when empty, ≥3000) | PR state, self-protection can be bypassed | Override label without permission check; approvals only on head SHA; no tests |
| B8 | [zeroclaw-labs/zeroclaw](https://github.com/zeroclaw-labs/zeroclaw) | implemented as report-only; approval rule only in docs | `.github/risk-labeler.yml` (content is JSON), own matcher; only `risk:high` configurable | low / medium / high | Maximum | medium; errors: exit 1 without a tier | Base (`pull_request_target`) | Evidence-based downgrade (diff only in test blocks); regex on added lines with path scope; trace with glob, rule name, line number; missing patch = high; report-only with mismatch output |
| B9 | [piggyway/piggy-frontend](https://github.com/piggyway/piggy-frontend) | implemented on `production` | `.github/risk-tiers.json`: `skip`, `low`, `high` groups, `trusted_authors`, `max_changed_lines`, `circuit_breaker_days` | low / medium / high | High before low; `skip` beats everything | medium (auto-merge); empty = low; truncated: job fails | Base SHA | Circuit breaker (a revert within 7 days stops auto-merge); review record as self-declaration in the PR body; no tests |
| B10 | [kompiro/karasu](https://github.com/kompiro/karasu) | implemented | Constants in `scripts/review/tier.ts` | low / design / medium / high + flag `securityReview` | Priority chain | medium (also when empty); unreadable path high | Base branch tip | Renames switched off so that moving never lowers; regex on lines (dangerous sinks); file status as a signal; record survives base merges with the same diff fingerprint; label from the author, tool only checks the floor |
| B11 | [FilipMarzuki/matlu](https://github.com/FilipMarzuki/matlu) | implemented | `.github/review-risk.json` with weights | low / medium / high via score | additive score (thresholds 3 and 6) | low with auto-merge (also when empty); truncation undetected | Default branch | `lens` per rule (review perspective), the union controls reviews; `content` with `line`/`within`; `exclude` per rule; gate recomputes instead of trusting the label; no human gate |
| B12 | [Habit130/agent-repo-governance](https://github.com/Habit130/agent-repo-governance) | implemented | strict YAML subset with `version` | declared, path floors medium/high | highest floor | no floor (declared tier applies); missing record: gate fails | Default branch tip | A mechanically proven revert lifts floors; waiver only by owner, bound to head SHA; review minimums checked after merge, `miss` issue; no auto-merge |
| B13 | [nhuthuynh/racket-analytics](https://github.com/nhuthuynh/racket-analytics) | implemented, no CI job | Data in the Python code | 0 / 1 / 2 | Maximum, unordered | 2 (also when empty); truncated: `unknown`, exit 2 | not verified | **Specialist flags = audiences** (roles per path, union, separate from the tier); liveness test per glob, `PENDING_GLOBS` with ticket; mode bit/shebang as a signal |
| B14 | [tetsuh/pi-tidd-agents](https://github.com/tetsuh/pi-tidd-agents) | proposal, rejected | – | Cliff / Hill / Meadow | Maximum | strictest | – | Reason for rejection: only ~7 % of PRs routed differently; docs are not low-risk there |
| B15 | [ggsegverse/.github](https://github.com/ggsegverse/.github) | implemented | `case` arms in the script + `extra-t2-paths` per package | T0 / T1 / T2 | First match | T1; empty = T0 | Default branch of the org repo | Packages may only tighten; T2 requires a green revdep check on head SHA; mandatory section "How to revert"; `@export` in the diff raises |
| B16 | [mimir-stellar/mimir-markets](https://github.com/mimir-stellar/mimir-markets) | implemented, but CI areas instead of risk | Prefixes in the script | app / agents / contracts / tests | Union | own state UNKNOWN, exit 1; empty = doc | PR head | Probe corpus with near misses (`appx/`, `docs2/`); parity check of two implementations |
| B17 | [gtrabanco/agentic-workflow](https://github.com/gtrabanco/agentic-workflow) | implemented, local CLI | Regexes in the code | passive / standard / high | First match | standard; empty = standard with reason `empty-change` | local | Separate dimension `size_tier`; capability analysis `destructive`; JSON `risk-tier-verdict@1` with signals |
| B18 | [ChanMeng666/archlang](https://github.com/ChanMeng666/archlang) | implemented, local | `RULES` in `scripts/risk-tier.mjs`: `[tier, regex, reason]` | low / medium / high | First match | medium; empty = low | local checkout | Tier is recomputed on landing and must equal the recorded one; path normalisation |
| B19 | [gabe4coding/claude-code-mode](https://github.com/gabe4coding/claude-code-mode) | `RISK.md` only read by a test; real rule set `.github/risk.yml`, advisory | `RISK.md` front matter + `.github/risk.yml` | T0 / T1 / T2-soft / T2-hard / HOLD | ? | T1 (also when empty) | Base branch | Lint "every glob matches" and "no file in two tiers"; workflow diff additive vs. weakening; removed assertions/skip markers; tier HOLD; kill switch |
| B20 | [norton77930/Provost](https://github.com/norton77930/Provost) | only policy text for the model | `REVIEW.md`, list "`glob`: R1|R2. Reason" | R1 / R2 as a floor | Maximum | – | – | No parser; automatic classifier explicitly a non-goal |
| B21 | [pimcabanlit/babala](https://github.com/pimcabanlit/babala) | document only | Table in `REVIEW.md`, paths as prose | Low / Medium / High | highest wins | "when in doubt, one tier higher" | – | Column "what does the human do per tier"; tier-dependent mandatory tools |
| B22 | [Nishfleet/fleet-ops](https://github.com/Nishfleet/fleet-ops) | only in a closed PR | `config/risky-paths.json`, regex | guard / held / green | any match | green; empty, ≥3000, unreadable, crash: fail closed | `main` of the central repo | 150 test cases in the rule file, run by CI against the classifier; `hold_no_patch`; approval via an unedited comment with head SHA |
| B23 | [OpenCara/OpenCara](https://github.com/OpenCara/OpenCara) | proposal | – | – | – | – | – | Nothing new |
| B24 | [arnelirobles/lean-agent](https://github.com/arnelirobles/lean-agent) | proposal; predecessor `needs-review.sh` in the sister repo | INI with `path`/`added`/`removed` | no tiers | Union | – | Head | Regex on removed lines; "not checkable" as a finding |
| B25 | [michaelloistl/skills](https://github.com/michaelloistl/skills) | proposal | `drive-spec.json` (planned) | auto / human | only raise | human | – | Shadow mode as the default; built-in, non-removable human paths; every commit author must be trusted |
| B26 | [northMES/northmes](https://github.com/northMES/northmes) | implemented | `.github/risk-paths`, CODEOWNERS dialect | risk:low / risk:high | any match | low only if all six conditions are met; >3000 high; error: job red | Default branch | Meta test CODEOWNERS ↔ rule file; `risk-refresh.yml` restarts the check after approval; invalid patterns (`!`, `[`) are errors |
| B27 | [jcasnellie69/homelab-config](https://github.com/jcasnellie69/homelab-config) | only in a feature branch | Bash `case` | A / B / C | Allowlist | C; empty = A with auto-merge | PR head | Allowlist model; workflow and CODEOWNERS contradict each other |
| B28 | [phucvinh57/biopass](https://github.com/phucvinh57/biopass) | implemented | Regex in the script | low / medium / high | high wins | medium (also when empty); ≥3000 high | Base SHA | Switch auto-merge off first, back on after successful classification |
| B29 | [r3dbars/transcripted](https://github.com/r3dbars/transcripted) | implemented | Python tuples, fnmatch | low / medium / high + `owner_required` | Category chain | high (also when empty, incomplete) | Default branch (`workflow_run`) | Check run by its own app, bound to PR+base+head, orphaned passes revoked; deleted tests and skip lists = high; size thresholds; kill switch; no override; 809 lines of tests |
| B30 | [hartz89/docent](https://github.com/hartz89/docent) | implemented, local CLI | Predicates in the code | 0 / 1 / 2a / 2b | First match | 2a; empty = 0; incomplete 2b | no binding | "Spans several packages" raises; backtest against old PRs; lowercasing before matching; quoted paths rejected; escaping against Markdown injection |
| B31 | brantsjohnson/Bridger-App | not accessible (404) | – | – | – | – | – | Not examined |
| B32 | [qomiralabs/drefyn](https://github.com/qomiralabs/drefyn) | implemented | `scripts/review-gate/rules.json`, typed matchers, strict | A / B / C via classes security/tests/ui/docs | Class order | B; empty = A; incomplete/invalid: status `error` | Base SHA; required workflow via org ruleset | **Roles per tier** (code, qa, security, design) with logins per role; approval only survives a byte-identical, conflict-free base merge; approval as a marker line with SHA |
| B33 | [schema-labs-ltd/babel-bridge](https://github.com/schema-labs-ltd/babel-bridge) | implemented | Globs in `risk-tier.mjs` | low / medium / high | high wins, low only if all are low | medium (also when empty); incomplete high | Base SHA | Approval via GitHub environment with required reviewers; test "every pattern has a positive example"; U+2028 hardening; weakness: tests never high |
| B34 | [mowan-labs/.github](https://github.com/mowan-labs/.github) | implemented | Policy with `deleted_paths`, `non_behavioral` | low / medium / high + axis "kind" | ? | medium; **fail-open bug not fixed** | Base SHA | LLM may lower the path tier (floor only `.github/`); `deleted_paths` as a rule type |
| B35 | [IvoryHeart/herdr-world](https://github.com/IvoryHeart/herdr-world) | proposal | – | Low / Normal / High | highest | – | – | Tier never drops within a PR; owner can pin it |
| B36 | [AnthonyPluth/Waypoint](https://github.com/AnthonyPluth/Waypoint) | implemented (logic in `AnthonyPluth/fleet`) | List of exact files | risky / not | any match | not risky | Base branch | Trimmed because a broad list made almost everything risky: only files that directly implement a security promise; CI lint of the list |
| B37 | [HeddleCo/heddle](https://github.com/HeddleCo/heddle) | proposal (issue open) | `.heddle/review-risk.toml` (planned) | low / elevated / high | – | – | not specified in the issue | Calibration via revert rate per tier and rule; "base rules on config change" is not in the issue |
| B38 | [gordonpn/merge-verdict](https://github.com/gordonpn/merge-verdict) | evaluator implemented, result controls nothing | Policy with freely named tiers | free, without ranking | per property the strictest | strictest combination of all tiers | Base SHA | Tiers as bundles of properties (approval, human review, effort), combined individually; docs claim an effect |
| B39 | [jblossey/houserules](https://github.com/jblossey/houserules) | partially implemented | `.houserules.json`, globset | Routes up to `senior-review` | Maximum | lowest tier never the default | Head (base named as open) | Rule/config changes fixed at `senior-review`; reasons also for rules without a finding; hash of the rule sources at merge base and head; sampling audit for the lowest tier |
| B40 | [Icewater1337/fride](https://github.com/Icewater1337/fride) | proposal (issue open) | – | 3 tiers, 0/1/2 approvals | – | – | – | Agent approvals never count as human; the person who gave the prompt is the responsible party |

## C. Products with a similar function

| # | Product | Approach | Notable |
|---|---|---|---|
| C1 | [Nx Owners](https://nx.dev/docs/reference/owners/overview) / [Conformance](https://20.nx.dev/nx-api/conformance) | Ownership in `nx.json`/`project.json`, compiled to CODEOWNERS | Matching via project tags instead of paths; `nx sync:check` in CI; requires a licence |
| C2 | [Dash0 AutoMerge](https://www.dash0.com/docs/dash0/darkplane/automerge/key-concepts) | Path rules → Protected / Dry run / Auto-approve / Auto-merge | "Dry run" as a tier for rollout |
| C3 | [CodeRabbit](https://docs.coderabbit.ai/pr-reviews/automatic-approvals) | Max. merge risk for approval, path/team overrides | PR branch cannot switch the permission back on; renames match old and new |
| C4 | [cubic](https://docs.cubic.dev/ai-review/auto-approval) | Allowlist + never-auto-approve denylist | Denylist always wins |
| C5 | [Greptile](https://www.greptile.com/docs/code-review/auto-approve-prs) | Risk ceiling + `includePaths` | All files must match |
| C6 | [Graphite Protections](https://graphite.dev/docs/protections) | Mergeability by path, language, team | Replacement for CODEOWNERS/rulesets |
| C7 | [Aviator FlexReview](https://docs.aviator.co/flexreview/how-to-guides/mandate-an-expert-approval) | Approval check by path | Mandatory expert per path |
| C8 | [Gitar](https://www.sonarsource.com/developers/blueprints/gitar-risk-triage-and-cross-pr-analysis/) | `.gitar/config/risk.md` in plain text | Rule names path, kind of change and reason (cf. `reason`) |
| C9 | [Unblocked](https://docs.getunblocked.com/code-review/risk-assessment) | Risk labels by agent | Labels as a query interface |
| C10 | [Cursor Approval Agents](https://cursor.com/docs/approval-agents) | PR routing and approval by risk | ? |
| C11 | Kubernetes/Prow and Chromium `OWNERS` | Distributed files per directory, inheritance upwards | Model for partial configs; `Owners-Override` as a regulated override |
| C12 | SonarQube | Hotspot priority by rule category | No path criticality |

## D. Documented bugs and pitfalls (for conformance tests)

| # | Pitfall | Source |
|---|---|---|
| D1 | Rename: a moved high file is evaluated only by its new path (missing `--no-renames`) | [Tim-van-Oudheusden/website #859](https://github.com/Tim-van-Oudheusden/website/issues/859) |
| D2 | Empty path set yields the lowest tier (fail open) | [klausi3D/godotGS #812](https://github.com/klausi3D/godotGS/issues/812) |
| D3 | File list truncated (GitHub API limit 3000) | A1, B6, B7, B13 |
| D4 | Git-quoted paths (non-ASCII, special characters) match no rule, missing `-z` | [MinimalUI #177](https://github.com/Tim-van-Oudheusden/MinimalUI/issues/177), [#193](https://github.com/Tim-van-Oudheusden/MinimalUI/issues/193) |
| D5 | A crash of the classifier lets a high PR through | B34 |
| D6 | `.env*` matches only in the root with minimatch; dotfiles need the `dot` option; case sensitivity | B1 |
| D7 | A broad low glob (`**/*_test.go`, `**/*.md`) pulls sensitive files down when "highest wins" is missing | B1 |
| D8 | Agent instruction files (`CLAUDE.md`) count as docs and bypass the security lane | CI lane PRs, e.g. [arcforgelabs/flow #498](https://github.com/arcforgelabs/flow/pull/498) |
| D9 | Default "medium" is later merged without review | Dailies #347 |
| D10 | Policy read from the PR head, PR loosens its own rules; `pull_request_target` risk | B5 |
| D11 | Non-blocking reviews are overtaken by the merge | B11 |
| D12 | Approvals on an old commit keep counting | A2, B5, B23 |

## E. Feature catalogue to go through

Candidates for the standard, with the implementations where they can be studied.

| Feature | Look at |
|---|---|
| Fail-closed default | B1, B2, B13, B24 |
| Highest tier wins / merge instead of first match | A1, A4, B1, B6 |
| Policy from the base ref, self-protection of the rule file | B2, B3, B4, B5, B22, A14 |
| Several label dimensions (risk + domain/area) | B8, B4, B13 |
| Audiences / specialists | B13, A9, B1 (QA vs. security) |
| Separation of classification ↔ decision | A12, B8 |
| Reason per rule | C8, B2 (sticky comment) |
| Exceptions in rules (`except`) | A4, C3 (`!` globs) |
| Minimum tier (floor) instead of a fixed tier; AI may only raise | B20, A13, A14 |
| Content-based escalation in addition to the path | B1 |
| Tier for the irreversible | A4 ("one-way-door") |
| Manual override, regulated | B8, B5 (admin on head SHA), C11 |
| Renames, empty/truncated list, special characters | D1–D4 |
| Schema versioning and migration | B3 |
| Org-wide reuse (central workflow) | B1, B3, B15, B34 |
| Monorepo: project tags, distributed configs | C1, C11 |
| Rollout mode (dry run, measurement phase) | C2, B27 |
| Maintenance effort of the rule list | B36 |
| Output as labels vs. status check vs. comment | B6, A2, B2 |
| Approvals bound to head SHA | A2, B5, B23 |

## F. Findings from the originals read (block A, B2–B5)

### Where the implementations diverge

| Decision | Variants | Who |
|---|---|---|
| Combination per file | Maximum of all matches | A1, A4, A6, B3 |
| | First match in rule order | A3, B2, B4 |
| | Path only as a floor under another signal | A2, A13 |
| `*` across `/` | yes, no `**` | A5, B2, B4, B5 |
| | no, `**` for depth | A1, A4, A6, A7, A9, B3 |
| Dotfiles | explicitly `dot: true` | A1, A6, A9 |
| Pattern without a slash | matches at any depth | A1 |
| | matches only in the root | A2 |
| Default for unmatched | mandatory field | A4, A6, B3 (v2) |
| | optional, default medium | A1, B2 |
| | fixed lowest tier | A3, A12 |
| | no default, PR fails | A2, A7 |
| Empty file list | lowest tier | A3, A4, A6, A12, B4 |
| | default tier | A1 |
| Truncated file list | highest tier or error | A1, A2, A6, A7, B3, B5 |
| | ignored (no pagination) | A8 |
| Policy source | Base SHA | A6, B3, B4 |
| | Base/target branch tip | A2, A5, A7, B5 |
| | Default branch tip | B2, A4 (auto-merge) |
| | PR head | A3, A12, A11 (under `pull_request`) |
| Self-protection of the rule file | hard-coded | A2, A6, B2, B3 (org floor) |
| | only via a rule in the file | A5, B4, B5 |
| | none | A1, A3, A4, A7 |
| Missing rule file | error / highest tier | A2, B5 |
| | silent defaults | A1, A3, A5, A11, B2, B3 |
| Unknown keys | error | A2, A4, A5, A6, A7, B3 |
| | ignored | A1 |
| Renames | both paths count | A1, A6, A7, A9, A13 |
| | old path invisible | A3, A11 |

### Candidates to adopt for the standard

| Building block | Best template | What exactly |
|---|---|---|
| Schema and validation | A4, A6, B3 | Mandatory fields version and default; unknown keys, duplicate globs, globs that can never match and negation as errors |
| Rule with reason | B3, B2 | `why` per rule as a mandatory field; in B3 with a category prefix |
| Exceptions | A4 | `except` per rule instead of global negation |
| Content rules | A4, A6 | Regex on changed lines; unreadable content counts as a match |
| Audiences | A9 | Teams with `all-of`/`any-of`; explicit `unmatched` rule |
| Layering, tighten only | B3, A2 | Org floor above the repo list; path as a floor under a label |
| Additional signals | A13, A6 | May only raise; an error of the signal counts as high |
| Trace output | A1, A6, A7 | Triggering glob with config line; typed reasons; JSON with a version field |
| Tooling | A7 | `validate`, offline scenario tests, simulation |
| Approval bound to head SHA | B5, A4, A2 | Approval with `commit_id == head`, stateless; `fresh`/`carry` per tier |
| Audit | A5 | Tier and approval as commit trailers, checkable after the fact |
| Org-wide use | B3 | Reusable workflow with SHA pin, list at the base SHA via the Contents API |
| Consistency test | B4 | Parity test between rule file and `labeler.yml` |

### What the standard must specify explicitly

1. Combination per file: maximum, not first match. Three of the implementations read use first match; there an early broad low rule beats a later high rule.
2. Glob dialect normative: `*` and `/`, `**`, dotfiles, patterns without a slash, case sensitivity, negation.
3. Four separate error cases with a fixed result: unmatched path, empty file list, truncated file list, missing or invalid rule file. Almost every implementation is fail open in at least one of them.
4. Policy source: base SHA or branch tip. Stacked PRs are the reason why B2 deviates from the base ref.
5. Self-protection in the format itself, not as a rule that someone can forget.
6. Path input NUL-separated, renames with both paths, deletions count.

## G. Findings from round 2 (block B complete)

### New features compared with round 1

| Feature | Where | Classification for the standard |
|---|---|---|
| Audiences as a separate axis next to the tier, union across matches | B13 (specialist flags), B11 (`lens`), B32 (roles per tier), A9 (teams) | Core; confirms `review: [...]` with union |
| Several axes instead of one ranking, per property the strictest | B38, B10 (`securityReview`), B29 (`owner_required`), B17 (`size_tier`) | Core; supports open labels |
| Layering that only tightens | B3 (org floor), B15 (`extra-t2-paths`), B25 (built-in human paths) | Core for monorepo partial configs |
| Test cases in or next to the rule file | B22 (150 cases), B16 (near misses), B30 (backtest of old PRs) | Optional `tests:` block plus conformance suite |
| Lint: every glob matches something | B13, B19, B33, B36 (four times independently) | Validator requirement |
| Lint: no file in contradictory tiers | B19 | Validator, as a hint under maximum semantics |
| Parity with CODEOWNERS / `labeler.yml` | B26, B4; negative example B27 | Generator instead of a second source (cf. A4, C1) |
| Regex on removed lines; deleted tests, skip markers | B24, B29, B19, B34 (`deleted_paths`) | Extension, outside the core |
| "Not checkable" as a finding of its own | B22, B24, B8, A6 | Core: a missing diff is never low |
| Own state for "unclassified" instead of a silent default | B16 (UNKNOWN), B17 (`empty-change`), B13 (`unknown`) | Core: the result carries the reason |
| Evidence-based downgrade | B8 (test blocks only), B12 (pure revert), B6 (verified bot) | Extension; only with machine proof |
| Report-only / shadow mode | B8, B25, C2 | Rollout path |
| Tier never drops within a PR; recomputation on merge | B35, B3 (ratchet), B18, B11 | Requirement for tools |
| Approval survives clean base merges | B32 (byte-identical merge), B10 (diff fingerprint) | Requirement for tools; important for product approval |
| Restart the check after approval | B26 | Requirement for tools |
| Approval channel | B33 (GitHub environment), B29 (own app), B22 (comment with SHA), B32 (marker line) | Options for "product approval" |
| Switch auto-merge off before classification | B28, B6 | Requirement for tools |
| Calibration via reverts | B9 (circuit breaker), B37 (revert rate per rule, proposal) | Accompanying process |
| "What does the human do per tier" | B21, A6 (`guide`) | Addition to `reason` for briefings |
| Path hardening | B30, B33, B18 | Conformance tests |

### Recurring mistakes

- **Empty file list yields the lowest tier** in B4, B9, B11, B15, B18, B27, B30, B32 as well as A3, A4, A6, A12. This is the most common fail-open case.
- **First match** in B2, B4, B7, B15, B17, B18, B30 and A3. There the order of the rules carries the security.
- **Markdown as a rule source is evaluated nowhere** (B19, B20, B21): only the model or a test reads it.
- **Docs claim more than the code** in A12, B38, B5, A11, B15.
- **Gate without effect:** only advisory or never wired up in B2, B3, B7, B8, B19, B38, A12.
- **Self-reported review records** (B9, B10) check form and freshness, not whether a review took place.
- **Map formats drift even within one org** (B1): the only parser does not understand the newer format of the neighbouring repos.

### Corrections to the sources of our own design decisions

- **HeddleCo/heddle #2114:** The issue only says "whoever touches the rule file is high". Evaluation with rules from the base branch is not stated there; it is evidenced in B3, B4, B9, B28, B32, B33, A6.
- **Tim-van-Oudheusden/Dailies #347:** It was fixed by adding high rules. The default for unmatched paths is still medium and is merged automatically.
- **jblossey/houserules #73:** Evaluation currently uses the rules from the head; base evaluation is named as open there.

### Counterargument

B14 (pi-tidd-agents) rejected risk tiers: only about 7 % of the last 40 PRs would have been routed differently. B36 (Waypoint) trimmed its list because a broad list made almost every change risky. Both speak for a backtest against old PRs before rollout.
