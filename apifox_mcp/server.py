"""Enterprise MCP server backed by the canonical Go CLI."""

from __future__ import annotations

import time
import uuid
from contextlib import asynccontextmanager
from datetime import timezone
from typing import Any, Literal

from mcp.server import MCPServer
from mcp.server.auth.provider import AccessToken, TokenVerifier
from mcp.server.auth.settings import AuthSettings
from mcp.server.mcpserver import Context
from mcp_types import ToolAnnotations

from . import __version__
from .audit import AuditEvent, AuditLogger
from .cli_gateway import CliGateway
from .errors import McpServiceError
from .models import ChangeKind, ChangePlanData, ErrorInfo, ResultMeta, ToolResult
from .plans import PlanStore
from .policy import WritePolicy
from .settings import Settings, Transport

READ_ONLY = ToolAnnotations(
    readOnlyHint=True,
    destructiveHint=False,
    idempotentHint=True,
    openWorldHint=True,
)
PLAN_ONLY = ToolAnnotations(
    readOnlyHint=True,
    destructiveHint=False,
    idempotentHint=True,
    openWorldHint=True,
)
APPLY_CHANGE = ToolAnnotations(
    readOnlyHint=False,
    destructiveHint=True,
    idempotentHint=False,
    openWorldHint=True,
)


class StaticTokenVerifier(TokenVerifier):
    """Small deployment adapter for a secret supplied by an enterprise gateway."""

    def __init__(self, expected_token: str, scopes: tuple[str, ...]) -> None:
        self.expected_token = expected_token
        self.scopes = list(scopes)

    async def verify_token(self, token: str) -> AccessToken | None:
        import secrets

        if not secrets.compare_digest(token, self.expected_token):
            return None
        return AccessToken(
            token="[REDACTED]",
            client_id="enterprise-gateway",
            scopes=self.scopes,
            subject="enterprise-agent",
        )


def _auth_configuration(settings: Settings) -> tuple[AuthSettings | None, TokenVerifier | None]:
    if settings.transport is not Transport.STREAMABLE_HTTP or not settings.http_bearer_token:
        return None, None
    issuer = settings.auth_issuer_url or f"http://{settings.host}:{settings.port}"
    resource = (
        settings.resource_server_url
        or f"http://{settings.host}:{settings.port}{settings.http_path}"
    )
    auth = AuthSettings(
        issuer_url=issuer,
        resource_server_url=resource,
        required_scopes=list(settings.auth_scopes),
    )
    return auth, StaticTokenVerifier(settings.http_bearer_token, settings.auth_scopes)


def _request_id() -> str:
    return "req_" + uuid.uuid4().hex


def _success(
    settings: Settings,
    *,
    request_id: str,
    tool: str,
    mode: str,
    data: dict[str, Any],
    duration_ms: int,
    cli_version: str | None,
) -> ToolResult:
    return ToolResult(
        ok=True,
        request_id=request_id,
        tool=tool,
        project_id=settings.project_label,
        mode=mode,
        data=data,
        meta=ResultMeta(duration_ms=duration_ms, cli_version=cli_version),
    )


def _failure(
    settings: Settings,
    *,
    request_id: str,
    tool: str,
    mode: str,
    error: McpServiceError,
    duration_ms: int,
    cli_version: str | None = None,
) -> ToolResult:
    return ToolResult(
        ok=False,
        request_id=request_id,
        tool=tool,
        project_id=settings.project_label,
        mode=mode,
        error=ErrorInfo(
            code=error.code,
            message=error.message,
            retryable=error.retryable,
            exit_code=error.exit_code,
            apifox_status=error.apifox_status,
            details=error.details,
        ),
        meta=ResultMeta(duration_ms=duration_ms, cli_version=cli_version),
    )


async def _record_best_effort(audit: AuditLogger, event: AuditEvent) -> None:
    try:
        await audit.record(event)
    except McpServiceError:
        # Read operations must not become unavailable solely because an optional audit sink failed.
        return


def create_server(
    settings: Settings,
    *,
    gateway: CliGateway | None = None,
    plans: PlanStore | None = None,
    audit: AuditLogger | None = None,
) -> MCPServer:
    gateway = gateway or CliGateway(settings)
    plans = plans or PlanStore(settings.plan_ttl_seconds)
    audit = audit or AuditLogger(settings.audit_log_path)
    policy = WritePolicy(settings.write_mode)
    auth, verifier = _auth_configuration(settings)

    @asynccontextmanager
    async def lifespan(_: MCPServer):
        try:
            yield {
                "settings": settings,
                "gateway": gateway,
                "plans": plans,
                "audit": audit,
            }
        finally:
            await audit.close()

    server = MCPServer(
        name="apifox-enterprise",
        title="Apifox Enterprise MCP",
        description="Plan, audit, and safely apply Apifox/OpenAPI documentation changes.",
        instructions=(
            "Use read tools to inspect the project. For changes, call apifox_change_plan first. "
            "Only call apifox_change_apply after reviewing the returned preview and plan ID."
        ),
        version=__version__,
        lifespan=lifespan,
        auth=auth,
        token_verifier=verifier,
    )

    async def run_read(
        *,
        tool: str,
        args: list[str],
        ctx: Context,
        expect_json: bool = True,
    ) -> ToolResult:
        request_id = _request_id()
        started = time.perf_counter()
        try:
            response = await gateway.run(args, expect_json=expect_json)
            version = await gateway.version()
            result = _success(
                settings,
                request_id=request_id,
                tool=tool,
                mode="read",
                data=response.data,
                duration_ms=response.duration_ms,
                cli_version=version,
            )
            await _record_best_effort(
                audit,
                AuditEvent(
                    request_id=request_id,
                    tool=tool,
                    mode="read",
                    project_id=settings.project_label,
                    outcome="success",
                    duration_ms=response.duration_ms,
                ),
            )
            return result
        except McpServiceError as error:
            duration_ms = int((time.perf_counter() - started) * 1000)
            await _record_best_effort(
                audit,
                AuditEvent(
                    request_id=request_id,
                    tool=tool,
                    mode="read",
                    project_id=settings.project_label,
                    outcome="error",
                    duration_ms=duration_ms,
                    error_code=error.code,
                    cli_exit_code=error.exit_code,
                ),
            )
            return _failure(
                settings,
                request_id=request_id,
                tool=tool,
                mode="read",
                error=error,
                duration_ms=duration_ms,
            )

    @server.tool(
        title="Check Apifox project connectivity",
        annotations=READ_ONLY,
        structured_output=True,
    )
    async def apifox_project_check(ctx: Context) -> ToolResult:
        """Check local configuration and connectivity without exposing credentials."""
        return await run_read(
            tool="apifox_project_check", args=["config", "check", "--json"], ctx=ctx
        )

    @server.tool(
        title="Get Apifox project overview",
        annotations=READ_ONLY,
        structured_output=True,
    )
    async def apifox_project_overview(ctx: Context, limit: int = 10) -> ToolResult:
        """Get endpoint, path, schema, and tag counts plus samples in one export."""
        if limit < 1 or limit > 100:
            error = McpServiceError("INVALID_INPUT", "limit must be between 1 and 100")
            return _failure(
                settings,
                request_id=_request_id(),
                tool="apifox_project_overview",
                mode="read",
                error=error,
                duration_ms=0,
            )
        return await run_read(
            tool="apifox_project_overview",
            args=["overview", "--limit", str(limit), "--json"],
            ctx=ctx,
        )

    @server.tool(title="List Apifox endpoints", annotations=READ_ONLY, structured_output=True)
    async def apifox_api_list(ctx: Context, keyword: str = "", limit: int = 50) -> ToolResult:
        """List HTTP endpoints with optional keyword filtering."""
        args = ["api", "list", "--limit", str(limit), "--json"]
        if keyword:
            args.extend(["--keyword", keyword])
        return await run_read(tool="apifox_api_list", args=args, ctx=ctx)

    @server.tool(title="Get one Apifox endpoint", annotations=READ_ONLY, structured_output=True)
    async def apifox_api_get(ctx: Context, method: str, path: str) -> ToolResult:
        """Get one endpoint by HTTP method and exact path."""
        return await run_read(
            tool="apifox_api_get",
            args=["api", "get", "--method", method.upper(), "--path", path, "--json"],
            ctx=ctx,
        )

    @server.tool(title="List Apifox schemas", annotations=READ_ONLY, structured_output=True)
    async def apifox_schema_list(ctx: Context, keyword: str = "", limit: int = 50) -> ToolResult:
        """List reusable data schemas."""
        args = ["schema", "list", "--limit", str(limit), "--json"]
        if keyword:
            args.extend(["--keyword", keyword])
        return await run_read(tool="apifox_schema_list", args=args, ctx=ctx)

    @server.tool(title="Get one Apifox schema", annotations=READ_ONLY, structured_output=True)
    async def apifox_schema_get(ctx: Context, name: str) -> ToolResult:
        """Get one reusable schema by name."""
        return await run_read(
            tool="apifox_schema_get",
            args=["schema", "get", name, "--json"],
            ctx=ctx,
        )

    @server.tool(title="List Apifox tags", annotations=READ_ONLY, structured_output=True)
    async def apifox_tag_list(ctx: Context) -> ToolResult:
        """List tags and their endpoint counts."""
        return await run_read(tool="apifox_tag_list", args=["tag", "list", "--json"], ctx=ctx)

    @server.tool(title="List endpoints under a tag", annotations=READ_ONLY, structured_output=True)
    async def apifox_tag_apis(ctx: Context, tag: str) -> ToolResult:
        """List endpoints assigned to one exact tag."""
        return await run_read(
            tool="apifox_tag_apis",
            args=["tag", "apis", "--tag", tag, "--json"],
            ctx=ctx,
        )

    @server.tool(title="Audit Apifox documentation", annotations=READ_ONLY, structured_output=True)
    async def apifox_audit(
        ctx: Context,
        scope: Literal["responses", "all-responses", "path-naming", "consistency"],
        method: str = "",
        path: str = "",
        tag: str = "",
        style: Literal["kebab-case", "snake_case", "camelCase"] = "kebab-case",
        show_complete: bool = False,
    ) -> ToolResult:
        """Audit response completeness, path naming, or project consistency."""
        args = ["audit", scope]
        if scope == "responses":
            if not method or not path:
                error = McpServiceError("INVALID_INPUT", "responses audit requires method and path")
                return _failure(
                    settings,
                    request_id=_request_id(),
                    tool="apifox_audit",
                    mode="read",
                    error=error,
                    duration_ms=0,
                )
            args.extend(["--method", method.upper(), "--path", path])
        elif scope == "all-responses":
            if tag:
                args.extend(["--tag", tag])
            if show_complete:
                args.append("--show-complete")
        elif scope == "path-naming":
            args.extend(["--style", style])
        args.append("--json")
        return await run_read(tool="apifox_audit", args=args, ctx=ctx)

    @server.tool(title="Export OpenAPI JSON", annotations=READ_ONLY, structured_output=True)
    async def apifox_export_openapi(
        ctx: Context,
        oas_version: Literal["3.0", "3.1"] = "3.1",
    ) -> ToolResult:
        """Export the complete project as OpenAPI JSON without writing a local file."""
        return await run_read(
            tool="apifox_export_openapi",
            args=["export-openapi", "--format", "JSON", "--oas-version", oas_version],
            ctx=ctx,
        )

    def change_commands(
        kind: ChangeKind,
        spec: dict[str, Any] | None,
        method: str,
        path: str,
        tags: list[str] | None,
        folder: str,
        folder_id: int | None,
        sync_folder: bool,
    ) -> tuple[list[str], list[str], dict[str, Any] | None]:
        if kind is ChangeKind.TAGS_REPLACE:
            if not method or not path or tags is None:
                raise McpServiceError(
                    "INVALID_INPUT", "tags_replace requires method, path, and tags"
                )
            operation: dict[str, Any] = {
                "method": method.upper(),
                "path": path,
                "tags": tags,
            }
            if folder:
                operation["folder"] = folder
            if folder_id is not None:
                operation["folder_id"] = folder_id
            if sync_folder:
                operation["sync_folder"] = True
            payload = {"operations": [operation]}
            base = ["tag", "replace-batch", "--file", "-"]
            return (
                [*base, "--dry-run", "--json"],
                [*base, "--json"],
                payload,
            )
        if kind in {ChangeKind.TAGS_REPLACE_BATCH, ChangeKind.FOLDER_MOVE_BATCH}:
            if spec is None:
                raise McpServiceError("INVALID_INPUT", f"{kind.value} requires spec")
            command = (
                ["tag", "replace-batch"]
                if kind is ChangeKind.TAGS_REPLACE_BATCH
                else ["folder", "move-batch"]
            )
            base = [*command, "--file", "-"]
            return ([*base, "--dry-run", "--json"], [*base, "--json"], spec)
        if kind is ChangeKind.FOLDER_DELETE_EMPTY:
            if spec is None:
                raise McpServiceError("INVALID_INPUT", "folder_delete_empty requires spec")
            base = ["folder", "delete-empty", "--file", "-"]
            return (
                [*base, "--dry-run", "--json"],
                [*base, "--confirm", "--json"],
                spec,
            )
        if spec is None:
            raise McpServiceError("INVALID_INPUT", f"{kind.value} requires spec")
        mapping: dict[ChangeKind, list[str]] = {
            ChangeKind.ENDPOINT_CREATE: ["api", "create"],
            ChangeKind.ENDPOINT_UPDATE: ["api", "update"],
            ChangeKind.ENDPOINT_UPSERT: ["api", "upsert"],
            ChangeKind.SCHEMA_CREATE: ["schema", "create"],
            ChangeKind.SCHEMA_UPDATE: ["schema", "update"],
            ChangeKind.APPLY_DOCS: ["apply-docs"],
            ChangeKind.GENERATE_CRUD: ["generate-crud"],
        }
        base = [*mapping[kind], "--file", "-"]
        return ([*base, "--dry-run"], [*base, "--json"], spec)

    @server.tool(
        title="Plan an Apifox change",
        annotations=PLAN_ONLY,
        structured_output=True,
    )
    async def apifox_change_plan(
        ctx: Context,
        kind: ChangeKind,
        spec: dict[str, Any] | None = None,
        method: str = "",
        path: str = "",
        tags: list[str] | None = None,
        folder: str = "",
        folder_id: int | None = None,
        sync_folder: bool = False,
    ) -> ToolResult:
        """Validate and dry-run a change, returning a short-lived immutable plan ID."""
        request_id = _request_id()
        started = time.perf_counter()
        try:
            policy.require_plan()
            dry_args, apply_args, stdin_payload = change_commands(
                kind, spec, method, path, tags, folder, folder_id, sync_folder
            )
            response = await gateway.run(dry_args, stdin_payload=stdin_payload)
            version = await gateway.version()
            preview = response.data
            record = await plans.create(
                kind=kind,
                args=apply_args,
                stdin_payload=stdin_payload,
                preview=preview,
                project_id=settings.project_id or "",
                cli_version=version,
            )
            plan_data = ChangePlanData(
                plan_id=record.plan_id,
                kind=record.kind,
                payload_sha256=record.payload_sha256,
                expires_at=record.expires_at.astimezone(timezone.utc).isoformat(),
                preview=record.preview,
            )
            await audit.record(
                AuditEvent(
                    request_id=request_id,
                    tool="apifox_change_plan",
                    mode="plan",
                    project_id=settings.project_label,
                    outcome="success",
                    duration_ms=response.duration_ms,
                    plan_id=record.plan_id,
                    payload_sha256=record.payload_sha256,
                )
            )
            return _success(
                settings,
                request_id=request_id,
                tool="apifox_change_plan",
                mode="plan",
                data=plan_data.model_dump(mode="json"),
                duration_ms=response.duration_ms,
                cli_version=version,
            )
        except McpServiceError as error:
            duration_ms = int((time.perf_counter() - started) * 1000)
            await _record_best_effort(
                audit,
                AuditEvent(
                    request_id=request_id,
                    tool="apifox_change_plan",
                    mode="plan",
                    project_id=settings.project_label,
                    outcome="error",
                    duration_ms=duration_ms,
                    error_code=error.code,
                ),
            )
            return _failure(
                settings,
                request_id=request_id,
                tool="apifox_change_plan",
                mode="plan",
                error=error,
                duration_ms=duration_ms,
            )

    @server.tool(
        title="Apply a reviewed Apifox change plan",
        annotations=APPLY_CHANGE,
        structured_output=True,
    )
    async def apifox_change_apply(ctx: Context, plan_id: str) -> ToolResult:
        """Apply one immutable plan. Real writes require server write mode ``apply``."""
        request_id = _request_id()
        started = time.perf_counter()
        record = None
        version = None
        try:
            policy.require_apply()
            version = await gateway.version()
            audit.ensure_write_ready()
            record = await plans.acquire(
                plan_id,
                project_id=settings.project_id or "",
                cli_version=version,
            )
            await audit.record(
                AuditEvent(
                    request_id=request_id,
                    tool="apifox_change_apply",
                    mode="apply",
                    project_id=settings.project_label,
                    outcome="started",
                    duration_ms=0,
                    plan_id=record.plan_id,
                    payload_sha256=record.payload_sha256,
                )
            )
            response = await gateway.run(record.args, stdin_payload=record.stdin_payload)
            await plans.finish(plan_id, success=True)
            await audit.record(
                AuditEvent(
                    request_id=request_id,
                    tool="apifox_change_apply",
                    mode="apply",
                    project_id=settings.project_label,
                    outcome="success",
                    duration_ms=response.duration_ms,
                    plan_id=record.plan_id,
                    payload_sha256=record.payload_sha256,
                )
            )
            return _success(
                settings,
                request_id=request_id,
                tool="apifox_change_apply",
                mode="apply",
                data={"plan_id": plan_id, "result": response.data},
                duration_ms=response.duration_ms,
                cli_version=version,
            )
        except McpServiceError as error:
            if record is not None:
                await plans.finish(plan_id, success=False, retryable=error.retryable)
            duration_ms = int((time.perf_counter() - started) * 1000)
            await _record_best_effort(
                audit,
                AuditEvent(
                    request_id=request_id,
                    tool="apifox_change_apply",
                    mode="apply",
                    project_id=settings.project_label,
                    outcome="error",
                    duration_ms=duration_ms,
                    error_code=error.code,
                    plan_id=plan_id,
                    cli_exit_code=error.exit_code,
                ),
            )
            return _failure(
                settings,
                request_id=request_id,
                tool="apifox_change_apply",
                mode="apply",
                error=error,
                duration_ms=duration_ms,
                cli_version=version,
            )

    return server
