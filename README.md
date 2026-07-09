# Apifox CLI

[![Go](https://img.shields.io/badge/Go-CLI-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Apifox](https://img.shields.io/badge/Apifox-OpenAPI-orange)](https://apifox.com/)

`apifox-cli` is now a Go command line tool plus an AI Skill for writing Apifox/OpenAPI documentation. The CLI replaces the old tool-call workflow: agents and scripts should call `apifox-cli` commands directly.

The main use case is not importing an existing OpenAPI file. The expected workflow is:

1. Read routes, handlers, DTOs, validators, and product requirements from the target project.
2. Write structured hidden JSON files such as `.apifox-docs.json`, `.apifox-endpoint.json`, `.apifox-crud.json`, or `.apifox-schema.json`.
3. Validate locally.
4. Run `--dry-run`.
5. Apply the generated documentation to Apifox through the CLI.

For Go projects, do not use Swagger marker comments as the primary API documentation strategy. Keep Go code focused on implementation and let AI author structured OpenAPI/Apifox specs through this CLI.

## Install

Homebrew:

```bash
brew tap iwen-conf/tap
brew trust iwen-conf/tap
brew install --cask apifox-cli
apifox-cli --version
```

From source:

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

## CLI Commands

Use the canonical command surface in new scripts: `api create|update|upsert`, `schema create|update`, `apply-docs`, and `generate-crud`. The older `create-endpoint`, `update-endpoint`, and `upsert-endpoint` commands are legacy aliases for compatibility.

Every command and subcommand supports `--help` without requiring credentials, files, or network access. JSON-file inputs accept `--file -` for stdin. Validation commands support `--json` with output shaped as `{"valid": bool, "errors": [...]}` and exit with code `1` when invalid. Discovery, audit, and write commands return structured `--json` output for scripts.

Direct endpoint/schema/folder deletion and folder creation are not currently supported as mutating operations; those commands explain the limitation. Prefer tags for folder-like organization.

Project and discovery:

```bash
apifox-cli config check
apifox-cli versions
apifox-cli api list --limit 20
apifox-cli api get --method GET --path /orders
apifox-cli schema list --limit 20
apifox-cli schema get Order
apifox-cli tag list
apifox-cli folder list
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

Endpoint specs must include a business `title`, `path`, `method`, `description`, structured request/response schemas, and real examples. Every schema field and parameter must include `description`.

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

OpenAPI import/export remains available for migration, backup, and compatibility work. It is not the primary AI authoring path.

```bash
apifox-cli export-openapi --format JSON --oas-version 3.1 -o .apifox-openapi.json
apifox-cli export-openapi --scope tags --tag 订单管理 --format YAML -o .apifox-orders.yaml
apifox-cli import-openapi --file .apifox-openapi.json --endpoint-overwrite-behavior AUTO_MERGE
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
