# AGENTS.md

## Project Direction

This project provides two first-class surfaces backed by one execution core:

- `apifox-mcp`: the enterprise, agent-facing MCP server.
- `apifox-cli`: the canonical Go execution engine and direct scripting interface.

The MCP server must delegate Apifox operations to the Go CLI through structured JSON. Do not add a second Python implementation of Apifox HTTP/OpenAPI behavior.

The primary Agent workflow is:

1. Inspect the project with read-only MCP tools.
2. Call `apifox_change_plan` to validate and dry-run a proposed change.
3. Review the structured preview and immutable plan ID.
4. Call `apifox_change_apply` only when the server is explicitly configured with `APIFOX_MCP_WRITE_MODE=apply`.

The direct CLI workflow for scripts and local authoring is:

1. Read the target project's routes, handlers, DTOs, validation rules, and product requirements.
2. Write structured hidden JSON files such as `.apifox-docs.json`, `.apifox-endpoint.json`, or `.apifox-crud.json`.
3. Run local validation with `apifox-cli validate-docs`, `validate-endpoint`, or `validate-crud`.
4. Run `--dry-run` before any write.
5. Apply the docs to Apifox with the Go CLI: `apifox-cli apply-docs`, `api upsert`, `schema create`, or `generate-crud`.

For broad `apply-docs` writes, use `--batch-size 15 --dry-run` to produce a batch plan, then apply with `--offset` and `--limit` batches. Inspect dry-run output before the real write.

Official Apifox OpenAPI import/export commands remain available for migration, backup, and compatibility tasks. They are not the main AI documentation authoring path.

The removed Python direct-HTTP tools are not a compatibility surface. Historical users should migrate to the enterprise MCP tools or call the Go CLI directly.

## MCP Rules

- MCP tools must return typed structured output with stable `ok`, `data`, `error`, request identity, and timing fields.
- Read, plan, and apply capabilities must be separate server-side policy levels; ToolAnnotations are hints, not authorization.
- Default `APIFOX_MCP_WRITE_MODE` to `plan`. A real write must consume a valid, unexpired, immutable plan.
- Pass JSON specs to `apifox-cli` through stdin. Never place tokens or generated specs in command-line arguments or temporary files.
- Keep stdio stdout reserved for MCP protocol frames. Logs and redacted audit events go to stderr or an explicit JSONL file.
- Streamable HTTP defaults to loopback. Non-loopback binding requires authentication plus allowed Host and Origin lists.
- Default tests must be Hermetic and use an in-memory MCP client or fake CLI. Live Apifox tests require a separate explicit profile and sandbox project.

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
- Use official Apifox OpenAPI import/export endpoints for real writes. The MCP facade may orchestrate CLI commands but must not duplicate their business behavior.

## Debugging And Observability

- Fail fast on malformed specs and missing credentials.
- Print dry-run payloads for mutating commands.
- Preserve exact Apifox error messages where possible.
- Add explicit logs or command output when behavior is hard to diagnose.

## Release

- Go CLI releases use GoReleaser via `.github/workflows/release.yml`.
- Enterprise MCP releases also build platform-specific wheels for Windows, Linux, and macOS on x64/ARM64. Each wheel must bundle exactly one matching `apifox-cli` binary and must not use the universal `py3-none-any` tag.
- Published wheels must pass `scripts/verify_bundled_wheel.py`; runtime lookup prefers `APIFOX_CLI_PATH`, then the bundled binary, then `PATH`.
- The scoped npm package `@yanzhang123/apifox-mcp` currently targets Windows x64 and bundles standalone `apifox-mcp.exe` plus `apifox-cli.exe`; runtime installation must not require Python, uv, or Go.
- npm registry publication is manual through `.github/workflows/npm-publish.yml`; never place an npm token in repository files or automatic pull-request workflows.
- Homebrew publishing targets `iwen-conf/homebrew-tap` and writes `Casks/apifox-cli.rb`.
- The source repository must have `HOMEBREW_TAP_GITHUB_TOKEN` configured as an Actions secret before pushing release tags.
- Release by pushing a `v*` tag from main after CI passes.
