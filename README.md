# Apifox Enterprise MCP + CLI

[![Go](https://img.shields.io/badge/Go-CLI-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Apifox](https://img.shields.io/badge/Apifox-OpenAPI-orange)](https://apifox.com/)

This repository provides an enterprise MCP server for Agents and a Go CLI for deterministic Apifox/OpenAPI operations. `apifox-mcp` is the typed, policy-controlled Agent surface; `apifox-cli` remains the single execution core and direct scripting interface.

The MCP server does not duplicate Apifox HTTP logic. It validates tool input, enforces read/plan/apply policy, invokes `apifox-cli` through stdin, returns structured results, and records redacted audit events.

The main use case is not importing an existing OpenAPI file. The expected workflow is:

1. Read routes, handlers, DTOs, validators, and product requirements from the target project.
2. Write structured hidden JSON files such as `.apifox-docs.json`, `.apifox-endpoint.json`, `.apifox-crud.json`, or `.apifox-schema.json`.
3. Validate locally.
4. Run `--dry-run`.
5. Apply the generated documentation to Apifox through the CLI.

For Go projects, do not use Swagger marker comments as the primary API documentation strategy. Keep Go code focused on implementation and let AI author structured OpenAPI/Apifox specs through this CLI.

## Install

### Windows x64: npm only

The Windows npm package bundles the Python MCP runtime and Go CLI as standalone executables.
Python, uv, and Go are not required at runtime. After the scoped package is published:

```powershell
npm install -g @yanzhang123/apifox-mcp
apifox-mcp --help
apifox-cli --version
```

For Cursor on Windows, point `mcp.json` at the installed command shim, never at an internal
`node_modules/@yanzhang123/apifox-mcp` path. Resolve the active shim after each NVM switch with
`where.exe apifox-mcp.cmd`; either use `apifox-mcp.cmd` when Cursor inherits that `PATH`, or use
the returned absolute `.cmd` path as `command`. Restart Cursor after changing Node/NVM versions
and verify `apifox-mcp --help` from the same environment before debugging MCP status.

The npm package currently targets Windows x64 only. After CI succeeds for a push to `main`,
`.github/workflows/npm-publish.yml` publishes a new `npm/package.json` version automatically.
Already-published versions are skipped, so every release PR must bump the version before merge.

Automatic publishing uses npm Trusted Publishing (GitHub OIDC), not a long-lived `NPM_TOKEN`.
Configure the package once at npmjs.com with GitHub user `12zhangyan`, repository `apifox-mcp`,
workflow `npm-publish.yml`, and the `npm publish` action allowed.

### Recommended: bundled MCP wheel

Release wheels include the matching Go CLI binary. End users do not need a Go toolchain or
`APIFOX_CLI_PATH`; install the wheel for the current OS/architecture and configure credentials.

```powershell
# Windows x64 example; replace VERSION with a published release version.
uv tool install "https://github.com/iwen-conf/apifox-mcp/releases/download/vVERSION/apifox_cli-VERSION-py3-none-win_amd64.whl"
apifox-mcp --help
```

Published wheel targets are Windows, Linux, and macOS on x64/ARM64. The MCP resolves the CLI in
this order: explicit `APIFOX_CLI_PATH`, CLI bundled in the wheel, then `apifox-cli` on `PATH`.

### Docker

Docker also packages both the MCP server and Go CLI, so the host does not need Go:

```bash
docker build -t apifox-mcp .
docker run --rm -i --env-file .env apifox-mcp
```

### Source development

Source checkouts do not commit generated binaries. Developers must build the CLI with Go or set
`APIFOX_CLI_PATH` to an existing binary:

```bash
uv sync --locked
go build -o ./bin/apifox-cli ./cmd/apifox-cli
export APIFOX_CLI_PATH="$PWD/bin/apifox-cli"
uv run apifox-mcp --help
```

Homebrew:

```bash
brew tap iwen-conf/tap
brew trust iwen-conf/tap
brew install --cask apifox-cli
apifox-cli --version
```

Build only the CLI from source:

```bash
go build -o ./bin/apifox-cli ./cmd/apifox-cli
./bin/apifox-cli --help
```

Or install from the checkout:

```bash
go install ./cmd/apifox-cli
apifox-cli --help
```

## Configure

Set credentials through environment variables, or pass them before the subcommand with `--token` and `--project-id`.

```bash
export APIFOX_TOKEN="your-token"
export APIFOX_PROJECT_ID="your-project-id"

apifox-cli config check
```

Use `--base-url` only when your Apifox deployment is not `https://api.apifox.com`.

MCP writes default to plan-only. A real apply requires an explicit server setting:

```bash
export APIFOX_MCP_WRITE_MODE=plan   # default: inspect and dry-run only
# export APIFOX_MCP_WRITE_MODE=apply # enable only in an approved environment
export APIFOX_MCP_CLI_TIMEOUT=120   # configurable write timeout in seconds
export APIFOX_MCP_READ_CACHE_TTL=300 # OpenAPI/index cache TTL in seconds
```

## Enterprise MCP Tools

Read-only discovery:

- `apifox_project_check`
- `apifox_project_overview` (lightweight design-index counts and bounded samples; exact OpenAPI endpoint/path/schema counts are populated after schema data is cached)
- `apifox_api_list`, `apifox_api_get`
- `apifox_schema_list`, `apifox_schema_get`
- `apifox_tag_list`, `apifox_tag_apis`
- `apifox_audit`, `apifox_export_openapi`

Discovery commands share a long-lived Go read session. Endpoint navigation and tags use the
lightweight endpoint index; endpoint reads, overview, tags, and audits avoid OpenAPI export, while
schema commands reuse one cached OpenAPI snapshot. Successful applies invalidate the read cache.
Structured MCP output recursively redacts credentials, sensitive examples, and project IDs. Exact
endpoint misses return `ok: false` with `API_NOT_FOUND`; paths without a leading slash are normalized
before lookup. Endpoint/schema lists return summaries instead of full descriptions.

Controlled changes:

1. Call `apifox_change_plan` with a change kind and structured spec.
2. Inspect its dry-run preview, payload hash, expiry, and plan ID.
3. Call `apifox_change_apply` with only that plan ID. The server rejects disabled, expired, changed, cross-project, busy, or already-consumed plans.

Supported change kinds are `endpoint_create`, `endpoint_update`, `endpoint_upsert`, `schema_create`, `schema_update`, `apply_docs`, `generate_crud`, `tags_replace`, `tags_replace_batch`, `folder_move_batch`, and `folder_delete_empty`.

`tags_replace` accepts optional `folder`, `folder_id`, or `sync_folder=true`. Tag/folder changes use the lightweight endpoint API and never call OpenAPI import. Batch specs use `{"operations":[{"method":"GET","path":"/orders","tags":["Orders"],"folder":"EAM/Orders"}]}`.

Start stdio (default):

```bash
uv run apifox-mcp
```

Streamable HTTP is opt-in and binds to loopback by default:

```bash
uv run apifox-mcp --transport streamable-http --host 127.0.0.1 --port 8000
```

Non-loopback HTTP refuses to start unless bearer verification, issuer/resource URLs, and allowed Host/Origin lists are configured.

## CLI Commands

Use the canonical command surface in new scripts: `api create|update|upsert`, `schema create|update`, `apply-docs`, and `generate-crud`. The older `create-endpoint`, `update-endpoint`, and `upsert-endpoint` commands are legacy aliases for compatibility.

Every command and subcommand supports `--help` without requiring credentials, files, or network access. JSON-file inputs accept `--file -` for stdin. Validation commands support `--json` with output shaped as `{"valid": bool, "errors": [...]}` and exit with code `1` when invalid. Discovery, audit, and write commands return structured `--json` output for scripts.

Direct endpoint/schema deletion is not supported. Endpoint folders support real list/create/move operations and guarded empty-folder deletion; real deletion requires `--confirm` and refuses non-empty subtrees.

Project and discovery:

```bash
apifox-cli config check
apifox-cli overview --limit 10 --json
apifox-cli versions
apifox-cli api list --limit 20
apifox-cli api get --method GET --path /orders
apifox-cli schema list --limit 20
apifox-cli schema get Order
apifox-cli tag list
apifox-cli folder list
apifox-cli folder create Orders --parent-id 12 --dry-run
```

Endpoint documentation:

```bash
apifox-cli endpoint-template --method POST -o .apifox-endpoint.json
apifox-cli validate-endpoint --file .apifox-endpoint.json
apifox-cli api upsert --file .apifox-endpoint.json --dry-run
apifox-cli api upsert --file .apifox-endpoint.json
```

Batch AI documentation:

```bash
apifox-cli docs-template -o .apifox-docs.json
apifox-cli validate-docs --file .apifox-docs.json
apifox-cli apply-docs --file .apifox-docs.json --dry-run
apifox-cli apply-docs --file .apifox-docs.json
```

Endpoint `update` and `upsert` entries may be partial: `path` and `method` identify the
endpoint, while only the supplied documentation fields are merged. Parameter lists support
`query_params`, `path_params`, `header_params`, and `cookie_params` (with `cookies` accepted as
an alias), plus generic OpenAPI-style `parameters` entries with `in: query|path|header|cookie`.
Create entries remain strict and require complete request/response documentation.
For a standalone partial endpoint file, validate with `validate-endpoint --update --file FILE`.

For broad writes, apply batches with `--offset` and `--limit`:

```bash
apifox-cli apply-docs --file .apifox-docs.json --batch-size 15 --dry-run
apifox-cli apply-docs --file .apifox-docs.json --offset 0 --limit 15
apifox-cli apply-docs --file .apifox-docs.json --offset 15 --limit 15
```

CRUD and schema documentation:

```bash
apifox-cli crud-template -o .apifox-crud.json
apifox-cli validate-crud --file .apifox-crud.json
apifox-cli generate-crud --file .apifox-crud.json --dry-run
apifox-cli generate-crud --file .apifox-crud.json

apifox-cli schema template -o .apifox-schema.json
apifox-cli schema create --file .apifox-schema.json --dry-run
apifox-cli schema create --file .apifox-schema.json
```

Maintenance and audits:

```bash
apifox-cli tag apis --tag 订单管理
apifox-cli tag add --method GET --path /orders --tag 订单管理 --tag 核心接口
apifox-cli tag add --method GET --path /orders --tag 订单管理 --folder EAM/订单管理 --dry-run
apifox-cli tag replace-batch --file .apifox-meta-batch.json --dry-run
apifox-cli folder move-batch --file .apifox-folder-batch.json --dry-run
apifox-cli folder delete-empty --all --dry-run
apifox-cli folder delete-empty --all --confirm --json
apifox-cli audit responses --method POST --path /orders
apifox-cli audit all-responses --tag 订单管理
apifox-cli audit path-naming --style kebab-case
apifox-cli audit consistency
```

Raw Apifox `/v1` fallback for official endpoints that are not wrapped yet:

```bash
apifox-cli request GET /versions --json
apifox-cli request POST /projects/123/export-openapi --data-file .apifox-export-payload.json
```

## AI Skill

The repository includes a Codex Skill at `skills/apifox-cli`. Use it when an AI agent needs to write or maintain Apifox/OpenAPI docs from source code.

The Skill instructs the agent to:

- Read application code and requirements first.
- Write hidden JSON specs instead of editing Swagger comments in Go code.
- Run `validate-*` locally.
- Run `--dry-run` before writing to Apifox.
- Use CLI commands such as `api upsert`, `schema create`, `generate-crud`, and `apply-docs`.

## JSON Spec Rules

Endpoint create specs must include a business `title`, `path`, `method`, `description`, structured request/response schemas, and real examples. Endpoint update/upsert specs require only `path` and `method`, plus the fields being changed. Every supplied schema field and parameter must include `description`.

Example `.apifox-endpoint.json`:

```json
{
  "title": "创建订单",
  "path": "/orders",
  "method": "POST",
  "description": "创建订单，需要用户已登录",
  "tags": ["订单管理"],
  "request_body_schema": {
    "type": "object",
    "properties": {
      "item_id": {"type": "integer", "description": "商品ID"},
      "quantity": {"type": "integer", "description": "购买数量"}
    },
    "required": ["item_id", "quantity"]
  },
  "request_body_example": {"item_id": 1001, "quantity": 2},
  "response_schema": {
    "type": "object",
    "properties": {
      "order_id": {"type": "integer", "description": "订单ID"},
      "status": {"type": "string", "description": "订单状态"}
    },
    "required": ["order_id", "status"]
  },
  "response_example": {"order_id": 90001, "status": "pending"}
}
```

## Import And Export

OpenAPI import/export remains available for migration, backup, and compatibility work. It is not the primary AI authoring path. Export includes Apifox extension fields such as `x-apifox-folder` by default; use `--exclude-apifox-extensions` only when a portable spec is required.

Use `-o` or `--output` for export files (`--file` is accepted only as a deprecated compatibility alias). Do not use a reduced OpenAPI document with `OVERWRITE_EXISTING` for routine documentation updates: omitted parameters, bodies, examples, tags, or folders can be removed. Prefer `apply-docs`, lightweight tag/folder commands, and `AUTO_MERGE`. Always read back changed endpoints because Apifox import counters do not prove persistence for every endpoint shape.

Known Apifox service limitations: exported `x-apifox-folder` and `tags` are not authoritative;
use `api get`, `tag list`, and `folder list` for metadata checks. Folderless/root-path endpoints
may report an update without persisting descriptions, and some apparently empty folders cannot
be deleted because hidden cases or descendants remain. Treat these as explicit manual/probe
cases. Monitor `components.schemas` counts and export size during bulk work because repeated
imports can cause schema growth.

```bash
apifox-cli export-openapi --format JSON --oas-version 3.1 -o .apifox-openapi.json
apifox-cli export-openapi --scope tags --tag 订单管理 --format YAML -o .apifox-orders.yaml
apifox-cli import-openapi --file .apifox-openapi.json --endpoint-overwrite-behavior AUTO_MERGE
apifox-cli import-openapi --file .apifox-openapi.json --update-folder-of-changed-endpoint
apifox-cli import-openapi --url https://example.com/openapi.yaml --prepend-base-path
apifox-cli import-postman --file .postman-collection.json
```

Preview mutating import/export requests before calling Apifox:

```bash
apifox-cli export-openapi --scope tags --tag 订单管理 --dry-run
apifox-cli import-openapi --file .apifox-openapi.json --print-payload
```

## Develop

```bash
go test ./...
go build -o /tmp/apifox-cli ./cmd/apifox-cli
/tmp/apifox-cli --help
/tmp/apifox-cli docs-template | /tmp/apifox-cli validate-docs --file -
```

Release uses GoReleaser and the Homebrew cask in `iwen-conf/homebrew-tap`.
