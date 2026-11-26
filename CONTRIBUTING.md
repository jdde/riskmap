# Contributing

## Branches

The repository follows [gitflow](https://nvie.com/posts/a-successful-git-branching-model/).

| Branch | Purpose | Created from | Merged into |
|---|---|---|---|
| `main` | released versions only; every commit is a release and gets a tag `vX.Y.Z` | – | – |
| `develop` | the next release; day-to-day work lands here | `main` (once) | – |
| `feature/<name>` | one new feature or change | `develop` | `develop` |
| `bugfix/<name>` | a fix for something not yet released | `develop` or a `release/*` branch | the branch it came from |
| `release/<version>` | preparing a release: version, last fixes, no new features | `develop` | `main` **and** back into `develop` |
| `hotfix/<version>` | an urgent fix for a released version | `main` | `main` **and** back into `develop` |

In practice:

```sh
git switch develop && git pull
git switch -c feature/partial-configs
# work, commit, push
gh pr create --base develop
```

## Releases

A release takes two steps, and only the second one publishes anything.

1. **Prepare.** The owner of the repository starts the workflow "Prepare release" (Actions tab, or `gh workflow run release-prepare.yml -f version=0.2.0`). It creates the branch `release/0.2.0` from `develop`, opens the pull request into `main` and starts the checks on it. Last fixes reach the branch through `bugfix/*` pull requests.
2. **Publish.** Merging that pull request starts the workflow "Publish release". It tags the merge commit with `v0.2.0`, builds the binaries for the six targets, creates the GitHub release with a `checksums.txt`, and opens the pull request that brings the release branch back into `develop`. Closing the pull request without merging publishes nothing.

The version has no leading `v` and follows [semantic versioning](https://semver.org). A version with a suffix, such as `0.2.0-rc.1`, becomes a pre-release. A `hotfix/<version>` branch merged into `main` is published the same way; create it by hand from `main`.

One setting of the repository is needed (Settings → Actions → General): "Allow GitHub Actions to create and approve pull requests". Do not switch on the automatic deletion of merged branches, or the pull request back into `develop` cannot be opened.

Nothing is pushed to `main` or `develop` directly. Every change arrives through a pull request.

## Checks on every pull request

| Check | Workflow | What it does |
|---|---|---|
| Tests (ubuntu-latest), Tests (macos-latest) | `ci.yml` | `gofmt`, `go vet`, `go test ./...` |
| Coverage | `ci.yml` | `sh scripts/coverage.sh`; fails below 99 % of statements |
| Build | `ci.yml` | compiles the six release targets |
| Branch rules | `gitflow.yml` | the pull request goes from an allowed branch into an allowed target (table above) |

A pull request opened by a workflow does not start these checks by itself. "Prepare release" therefore starts both workflows by hand for the release branch; later pushes to the branch start them as usual.

Run the same checks before pushing:

```sh
gofmt -l .
go vet ./...
go test ./...
sh scripts/coverage.sh
```

New code needs tests in the same pull request, otherwise the coverage check fails.
