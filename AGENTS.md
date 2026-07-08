# AGENTS.md

## Project Direction

This branch turns `apifox-mcp` into a first-class Go CLI plus Codex Skill for AI-authored Apifox/OpenAPI documentation. The CLI replaces the old tool-call workflow; agents should call command-line subcommands directly.

The primary workflow is:

1. Read the target project's routes, handlers, DTOs, validation rules, and product requirements.
2. Write structured hidden JSON files such as `.apifox-docs.json`, `.apifox-endpoint.json`, or `.apifox-crud.json`.
3. Run local validation with `apifox-mcp validate-docs`, `validate-endpoint`, or `validate-crud`.
4. Run `--dry-run` before any write.
5. Apply the docs to Apifox with the Go CLI: `apifox-mcp apply-docs`, `api upsert`, `schema create`, or `generate-crud`.

Official Apifox OpenAPI import/export commands remain available for migration, backup, and compatibility tasks. They are not the main AI documentation authoring path.

## Go Documentation Rule

For Go projects, do not make Swagger marker comments the primary API documentation strategy. Go code should stay focused on implementation, routing, DTOs, and validation. Let AI extract API semantics from those sources and write structured Apifox/OpenAPI JSON through the CLI.

## CLI Rules

- The CLI implementation lives in `cmd/apifox-mcp` and must remain Go.
- Validate locally before calling Apifox.
- Do not hide validation or API errors behind fallback logic.
- Prefer hidden JSON files for AI-generated request/spec data.
- Keep CLI commands composable and scriptable.
- Use official Apifox OpenAPI import/export endpoints for real writes. Do not reintroduce a Python CLI wrapper or MCP tool-call path for the main CLI workflow.

## Debugging And Observability

- Fail fast on malformed specs and missing credentials.
- Print dry-run payloads for mutating commands.
- Preserve exact Apifox error messages where possible.
- Add explicit logs or command output when behavior is hard to diagnose.

## Release

- Go CLI releases use GoReleaser via `.github/workflows/release.yml`.
- Homebrew publishing targets `iwen-conf/homebrew-tap` and writes `Casks/apifox-mcp.rb`.
- The source repository must have `HOMEBREW_TAP_GITHUB_TOKEN` configured as an Actions secret before pushing release tags.
- Release by pushing a `v*` tag from main after CI passes.
