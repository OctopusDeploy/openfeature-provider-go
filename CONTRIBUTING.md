# Contributing

## Setup

You need Go 1.26 or later, the version in the `go` line of `go.mod`. Linting uses [golangci-lint](https://golangci-lint.run/) v2. CI uses the version pinned in `.github/workflows/main.yml`.

## Running the checks

CI runs these on every PR, and fails the build if any of them fail. Run them locally before you push.

| Check | Command | Fix it with |
|---|---|---|
| Build | `go build ./...` | |
| Vet | `go vet ./...` | |
| Tests | `go test ./...` | |
| Tidy modules | `go mod tidy -diff` | `go mod tidy` |
| Linting and formatting (golangci-lint) | `golangci-lint run` | `golangci-lint fmt` for formatting |
| Known vulnerabilities ([govulncheck](https://go.dev/doc/tutorial/govulncheck)) | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | Maintainers handle these. See [MAINTAINING.md](MAINTAINING.md) |

CI runs the tests on both the oldest Go version `go.mod` allows and the latest release.

## Dependencies

`go.mod` and `go.sum` are committed, and CI fails if they aren't tidy. After changing dependencies, run `go mod tidy` and commit the result.

Keep dependencies to a minimum. The provider is a library that customers embed, so every dependency ends up in their builds and their vulnerability scanners.

## Security vulnerabilities

Don't open a public issue for a vulnerability in the provider. Follow our [security policy](https://github.com/OctopusDeploy/.github/blob/main/SECURITY.md) instead.

Maintainers deal with vulnerable dependencies. If you're curious how, see [MAINTAINING.md](MAINTAINING.md).

## PR titles

PR titles must follow [Conventional Commits](https://www.conventionalcommits.org/), for example `feat: add flag caching`, and a check validates them. PRs are squash merged, so the title becomes the commit message on `main`. Release Please uses it to work out the next version and write the changelog.

Don't edit `version.go` or `CHANGELOG.md` by hand. Release Please manages both.

## Releases

Maintainers handle releases. If you're curious how it works, see [RELEASING.md](RELEASING.md).
