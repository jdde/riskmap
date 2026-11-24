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

A release:

```sh
git switch -c release/0.1.0 develop
# pull request release/0.1.0 -> main, merge, then tag main with v0.1.0
# pull request release/0.1.0 -> develop, to bring the last fixes back
```

Nothing is pushed to `main` or `develop` directly. Every change arrives through a pull request.

## Checks on every pull request

| Check | Workflow | What it does |
|---|---|---|
| Tests (ubuntu-latest), Tests (macos-latest) | `ci.yml` | `gofmt`, `go vet`, `go test ./...` |
| Coverage | `ci.yml` | `sh scripts/coverage.sh`; fails below 99 % of statements |
| Build | `ci.yml` | compiles the six release targets |
| Branch rules | `gitflow.yml` | the pull request goes from an allowed branch into an allowed target (table above) |

Run the same checks before pushing:

```sh
gofmt -l .
go vet ./...
go test ./...
sh scripts/coverage.sh
```

New code needs tests in the same pull request, otherwise the coverage check fails.
