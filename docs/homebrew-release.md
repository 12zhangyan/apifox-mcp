# Homebrew Release

This project publishes the Go CLI through a Homebrew tap.

## Tap

Default tap repository:

```text
iwen-conf/homebrew-tap
```

GoReleaser writes the cask to:

```text
Casks/apifox-mcp.rb
```

## One-time setup

1. Create or reuse `https://github.com/iwen-conf/homebrew-tap`.
2. Create a GitHub token that can push to `iwen-conf/homebrew-tap`.
3. Add the token to `iwen-conf/apifox-mcp` repository secrets as:

```text
HOMEBREW_TAP_GITHUB_TOKEN
```

The built-in `GITHUB_TOKEN` is enough for creating releases in `iwen-conf/apifox-mcp`, but it cannot push to a separate tap repository.

## Release

Tag a release from the main branch:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

The `release` GitHub Actions workflow will:

1. Build `apifox-mcp` for macOS and Linux, amd64 and arm64.
2. Upload archives and checksums to GitHub Releases.
3. Update `iwen-conf/homebrew-tap/Casks/apifox-mcp.rb`.

## Install

After the release workflow completes:

```bash
brew tap iwen-conf/tap
brew trust iwen-conf/tap
brew install --cask apifox-mcp
apifox-mcp --version
```

Homebrew 6 may refuse third-party casks from untrusted taps. If that happens, run `brew trust iwen-conf/tap` once before installing.

## Local Checks

```bash
go test ./...
go build -o /tmp/apifox-mcp ./cmd/apifox-mcp
/tmp/apifox-mcp docs-template | /tmp/apifox-mcp validate-docs --file -
```

If GoReleaser is installed:

```bash
goreleaser check
goreleaser release --snapshot --clean
```
