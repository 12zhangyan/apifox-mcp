# AGENTS.md

## Project Direction

This branch turns `apifox-cli` into a first-class Go CLI plus Codex Skill for AI-authored Apifox/OpenAPI documentation. The CLI replaces the old tool-call workflow; agents should call command-line subcommands directly.

The primary workflow is:

1. Read the target project's routes, handlers, DTOs, validation rules, and product requirements.
2. Write structured hidden JSON files such as `.apifox-docs.json`, `.apifox-endpoint.json`, or `.apifox-crud.json`.
3. Run local validation with `apifox-cli validate-docs`, `validate-endpoint`, or `validate-crud`.
4. Run `--dry-run` before any write.
5. Apply the docs to Apifox with the Go CLI: `apifox-cli apply-docs`, `api upsert`, `schema create`, or `generate-crud`.

For broad `apply-docs` writes, use `--batch-size 15 --dry-run` to produce a batch plan, then apply with `--offset` and `--limit` batches. Inspect dry-run output before the real write.

Official Apifox OpenAPI import/export commands remain available for migration, backup, and compatibility tasks. They are not the main AI documentation authoring path.

The old Python MCP package is legacy compatibility surface only. Do not use it for the primary AI documentation workflow.

## Go Documentation Rule

For Go projects, do not make Swagger marker comments the primary API documentation strategy. Go code should stay focused on implementation, routing, DTOs, and validation. Let AI extract API semantics from those sources and write structured Apifox/OpenAPI JSON through the CLI.

## CLI Rules

- The CLI implementation lives in `cmd/apifox-cli` and must remain Go.
- Prefer the canonical command surface: `api create|update|upsert`, `schema create|update`, `apply-docs`, and `generate-crud`.
- Treat `create-endpoint`, `update-endpoint`, and `upsert-endpoint` as legacy aliases for compatibility only.
- Validate locally before calling Apifox.
- Every command and subcommand should answer `--help` without requiring credentials, files, or network access.
- JSON-file inputs should accept `--file -` for stdin where applicable.
- Validation commands should support `--json` with `{"valid": bool, "errors": [...]}` and exit code `1` when invalid.
- Discovery and audit commands should provide structured `--json` output for scripts instead of plain text wrapped in `result`.
- Write commands should provide structured `--json` output with identity fields, counters, and raw `import_result` for scripts.
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
- Homebrew publishing targets `iwen-conf/homebrew-tap` and writes `Casks/apifox-cli.rb`.
- The source repository must have `HOMEBREW_TAP_GITHUB_TOKEN` configured as an Actions secret before pushing release tags.
- Release by pushing a `v*` tag from main after CI passes.
