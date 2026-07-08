---
name: apifox-cli
description: Write and maintain Apifox/OpenAPI documentation through the apifox-mcp CLI. Use when Codex needs to read source code, routes, DTOs, validation rules, or product requirements and author structured .apifox-docs.json, .apifox-endpoint.json, or .apifox-crud.json files; validate and dry-run them; apply them to Apifox; inspect existing Apifox endpoints/schemas; generate CRUD docs; or audit API response and naming consistency without relying on Go Swagger marker comments or an MCP client.
---

# Apifox CLI

Use the repository's Go `apifox-mcp` CLI as the source of truth. It talks to Apifox directly through official OpenAPI import/export APIs and related `/v1` endpoints.

```bash
apifox-mcp list-tools
apifox-mcp describe <tool>
apifox-mcp versions
apifox-mcp docs-template -o .apifox-docs.json
apifox-mcp validate-docs --file .apifox-docs.json
apifox-mcp apply-docs --file .apifox-docs.json --dry-run
apifox-mcp apply-docs --file .apifox-docs.json
apifox-mcp endpoint-template --method POST -o .apifox-endpoint.json
apifox-mcp validate-endpoint --file .apifox-endpoint.json
apifox-mcp upsert-endpoint --file .apifox-endpoint.json --dry-run
apifox-mcp upsert-endpoint --file .apifox-endpoint.json
apifox-mcp request GET /versions --json
apifox-mcp call <tool> --args '{"key":"value"}'
apifox-mcp call <tool> --args-file .apifox-request.json
```

## Setup

Verify configuration before making changes:

```bash
apifox-mcp call check_apifox_config
```

Resolve the CLI command before doing work:

- Prefer `apifox-mcp` when it is installed in PATH.
- When working inside this repository checkout, use `go run ./cmd/apifox-mcp ...` if the binary is unavailable.
- When the user provides an `apifox-mcp` checkout path, use `go run <path>/cmd/apifox-mcp ...` or build it with `go build -o <path>/bin/apifox-mcp <path>/cmd/apifox-mcp`.
- Do not use `python -m apifox_mcp.cli`; the main CLI is Go. Python is retained for the MCP server only.
- If no usable Go CLI is available, fail with the exact command-not-found, Go toolchain, or build error instead of guessing another API.

Require `APIFOX_TOKEN` and `APIFOX_PROJECT_ID`, or pass `--token` and `--project-id` before the subcommand. Use `--base-url` only when the project uses a non-default Apifox host.

For multi-field inputs, prefer hidden JSON files such as `.apifox-request.json` and call them with `--args-file`. Keep credentials in environment variables instead of JSON files.

## AI Documentation Workflow

For Go projects, do not make Swagger marker comments the primary documentation path. Read routes, handlers, DTOs, validation code, and product requirements; then write structured Apifox JSON specs and send them through the CLI.

Batch documentation flow:

```bash
apifox-mcp docs-template -o .apifox-docs.json
apifox-mcp validate-docs --file .apifox-docs.json
apifox-mcp apply-docs --file .apifox-docs.json --dry-run
apifox-mcp apply-docs --file .apifox-docs.json
```

Use `.apifox-docs.json` as the normal AI handoff format when documenting more than one endpoint or a resource family. It can contain:

- `endpoints`: endpoint specs, each with optional `action` set to `upsert`, `create`, or `update`; prefer `upsert` for documentation sync.
- `crud`: CRUD resource specs that are passed to `generate_crud_apis`.

Single endpoint flow:

```bash
apifox-mcp endpoint-template --method POST -o .apifox-endpoint.json
apifox-mcp validate-endpoint --file .apifox-endpoint.json
apifox-mcp upsert-endpoint --file .apifox-endpoint.json --dry-run
apifox-mcp upsert-endpoint --file .apifox-endpoint.json
```

Use `create-endpoint` only when the endpoint must be new. Use `update-endpoint` when the endpoint must already be treated as an overwrite. Use `upsert-endpoint` for normal AI documentation sync because the underlying Apifox import can create or update.

CRUD flow:

```bash
apifox-mcp crud-template -o .apifox-crud.json
apifox-mcp validate-crud --file .apifox-crud.json
apifox-mcp generate-crud --file .apifox-crud.json --dry-run
apifox-mcp generate-crud --file .apifox-crud.json
```

Keep generated specs as hidden JSON files (`.apifox-*.json`) while iterating. Always validate before writing to Apifox, and use `--dry-run` before the first real write.

## Discovery

List available operations when unsure:

```bash
apifox-mcp list-tools
apifox-mcp describe create_api_endpoint
```

Common read operations:

```bash
apifox-mcp call list_api_endpoints --param limit=20
apifox-mcp call get_api_endpoint_detail --param path=/orders --param method=GET
apifox-mcp call list_schemas --param limit=20
apifox-mcp call get_schema_detail --param name=Order
apifox-mcp call list_tags
apifox-mcp call list_folders
```

Use `apifox-mcp versions` to verify the configured `X-Apifox-Api-Version` remains supported. Use `apifox-mcp request METHOD /path` only for official Apifox `/v1` endpoints that are not yet wrapped by a high-level command.

## Import And Export

Use official Apifox import/export APIs for migration, backup, or compatibility work. Do not make this the primary AI documentation workflow:

```bash
apifox-mcp export-openapi --format JSON --oas-version 3.1 -o .apifox-openapi.json
apifox-mcp export-openapi --scope tags --tag 订单管理 --format YAML -o .apifox-orders.yaml
apifox-mcp import-openapi --file .apifox-openapi.json --endpoint-overwrite-behavior AUTO_MERGE
apifox-mcp import-openapi --url https://example.com/openapi.yaml --prepend-base-path
apifox-mcp import-postman --file .postman-collection.json
```

Use `--scope endpoints --endpoint-id ...`, `--scope tags --tag ...`, or `--scope folders --folder-id ...` to limit exports. Use repeated or comma-separated values for endpoint IDs, folder IDs, tags, excluded tags, and environment IDs.

Before mutating Apifox, preview the exact endpoint and payload:

```bash
apifox-mcp export-openapi --scope tags --tag 订单管理 --dry-run
apifox-mcp import-openapi --file .apifox-openapi.json --print-payload
apifox-mcp import-postman --file .postman-collection.json --dry-run
```

For imports, choose overwrite behavior deliberately:

- `OVERWRITE_EXISTING`: replace matched resources.
- `AUTO_MERGE`: merge into matched resources where Apifox supports it.
- `KEEP_EXISTING`: skip matched resources.
- `CREATE_NEW`: keep existing resources and create new ones.

## API Definition Rules

When creating or updating endpoints:

- Use Chinese business names for `title`; do not use method/path strings or role prefixes.
- Keep `description` as business context and metadata, not request/response examples.
- Put every JSON Schema in structured `response_schema` or `request_body_schema`.
- Include `description` on every schema field and parameter.
- Use real example values; do not use type placeholders such as `"string"`.
- Let the CLI add standard error responses unless the user explicitly supplies a custom `responses` list.

Example:

```bash
apifox-mcp call create_api_endpoint --args-file .apifox-create-order.json
```

Example `.apifox-create-order.json` shape:

```json
{
  "title": "创建订单",
  "path": "/orders",
  "method": "POST",
  "description": "创建新订单，需要用户已登录",
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

## Audit And Repair

Use audits before broad edits and after endpoint changes:

```bash
apifox-mcp call audit_all_api_responses
apifox-mcp call check_api_responses --param path=/orders --param method=POST
apifox-mcp call check_path_naming_convention --param style=kebab-case
apifox-mcp call check_response_consistency
```

If a command returns an error, report the exact CLI output and gather more context with `describe`, list/detail commands, or audits before retrying. Do not hide failures behind local assumptions.
