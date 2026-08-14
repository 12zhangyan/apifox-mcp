from __future__ import annotations

import asyncio
import io
from dataclasses import replace
from typing import Any

from mcp import Client

from apifox_mcp.audit import AuditLogger
from apifox_mcp.cli_gateway import GatewayResponse
from apifox_mcp.server import create_server
from apifox_mcp.settings import WriteMode

from .conftest import make_settings


class FakeGateway:
    def __init__(self) -> None:
        self.calls: list[tuple[tuple[str, ...], dict[str, Any] | None]] = []

    async def version(self) -> str:
        return "apifox-cli test-version"

    async def close(self) -> None:
        return None

    async def run(
        self,
        args: list[str] | tuple[str, ...],
        *,
        stdin_payload: dict[str, Any] | None = None,
        expect_json: bool = True,
    ) -> GatewayResponse:
        self.calls.append((tuple(args), stdin_payload))
        if "--dry-run" in args:
            return GatewayResponse({"operations": [{"action": "upsert"}]}, 2)
        if tuple(args[:2]) == ("api", "list"):
            return GatewayResponse({"endpoints": [], "total": 0}, 1)
        if args and args[0] == "overview":
            return GatewayResponse(
                {
                    "counts": {"endpoints": 2, "paths": 1, "schemas": 1, "tags": 1},
                    "samples": {"endpoints": [], "schemas": [], "tags": []},
                },
                1,
            )
        return GatewayResponse({"result": "ok"}, 1)


class ReadGateway(FakeGateway):
    def __init__(self, responses: dict[tuple[str, ...], dict[str, Any]]) -> None:
        super().__init__()
        self.responses = responses

    async def run_read(self, args: list[str] | tuple[str, ...]) -> GatewayResponse:
        self.calls.append((tuple(args), None))
        return GatewayResponse(self.responses[tuple(args)], 1)

    async def close(self) -> None:
        return None


def test_explicit_openapi_export_bypasses_restricted_read_session() -> None:
    async def scenario() -> None:
        gateway = ReadGateway({})
        server = create_server(
            make_settings(), gateway=gateway, audit=AuditLogger(stream=io.StringIO())
        )
        async with Client(server) as client:
            result = await client.call_tool("apifox_export_openapi", {})
            assert result.structured_content["ok"] is True
        assert gateway.calls == [
            (("export-openapi", "--format", "JSON", "--oas-version", "3.1"), None)
        ]

    asyncio.run(scenario())


def test_tool_discovery_and_structured_read_result() -> None:
    async def scenario() -> None:
        gateway = FakeGateway()
        server = create_server(
            make_settings(), gateway=gateway, audit=AuditLogger(stream=io.StringIO())
        )
        async with Client(server) as client:
            tools = (await client.list_tools()).tools
            names = {tool.name for tool in tools}
            assert "apifox_project_overview" in names
            assert "apifox_api_list" in names
            assert "apifox_change_plan" in names
            assert "apifox_change_apply" in names
            read_tool = next(tool for tool in tools if tool.name == "apifox_api_list")
            assert read_tool.annotations is not None
            assert read_tool.annotations.read_only_hint is True

            result = await client.call_tool("apifox_api_list", {"limit": 5})
            assert result.structured_content is not None
            assert result.structured_content["ok"] is True
            assert result.structured_content["data"]["total"] == 0

            overview = await client.call_tool("apifox_project_overview", {"limit": 5})
            assert overview.structured_content["ok"] is True
            assert overview.structured_content["data"]["counts"]["endpoints"] == 2

    asyncio.run(scenario())


def test_plan_only_mode_prevents_apply() -> None:
    async def scenario() -> None:
        gateway = FakeGateway()
        settings = make_settings(write_mode=WriteMode.PLAN)
        server = create_server(settings, gateway=gateway, audit=AuditLogger(stream=io.StringIO()))
        async with Client(server) as client:
            planned = await client.call_tool(
                "apifox_change_plan",
                {
                    "kind": "endpoint_upsert",
                    "spec": {"title": "创建订单", "method": "POST", "path": "/orders"},
                },
            )
            assert planned.structured_content["ok"] is True
            plan_id = planned.structured_content["data"]["plan_id"]

            applied = await client.call_tool("apifox_change_apply", {"plan_id": plan_id})
            assert applied.structured_content["ok"] is False
            assert applied.structured_content["error"]["code"] == "APPLY_DISABLED"
            real_writes = [
                args for args, _ in gateway.calls if "--json" in args and "--dry-run" not in args
            ]
            assert real_writes == []

    asyncio.run(scenario())


def test_apply_mode_consumes_plan_once() -> None:
    async def scenario() -> None:
        gateway = FakeGateway()
        settings = replace(make_settings(), write_mode=WriteMode.APPLY)
        server = create_server(settings, gateway=gateway, audit=AuditLogger(stream=io.StringIO()))
        async with Client(server) as client:
            planned = await client.call_tool(
                "apifox_change_plan",
                {
                    "kind": "schema_create",
                    "spec": {"name": "Order", "type": "object", "properties": {}},
                },
            )
            plan_id = planned.structured_content["data"]["plan_id"]
            first = await client.call_tool("apifox_change_apply", {"plan_id": plan_id})
            second = await client.call_tool("apifox_change_apply", {"plan_id": plan_id})
            assert first.structured_content["ok"] is True
            assert second.structured_content["error"]["code"] == "PLAN_CONSUMED"

    asyncio.run(scenario())


def test_tags_replace_uses_real_dry_run_and_frozen_stdin_payload() -> None:
    async def scenario() -> None:
        gateway = FakeGateway()
        settings = replace(make_settings(), write_mode=WriteMode.APPLY)
        server = create_server(settings, gateway=gateway, audit=AuditLogger(stream=io.StringIO()))
        async with Client(server) as client:
            planned = await client.call_tool(
                "apifox_change_plan",
                {
                    "kind": "tags_replace",
                    "method": "get",
                    "path": "/orders",
                    "tags": ["Orders"],
                    "folder": "EAM/Orders",
                },
            )
            assert planned.structured_content["ok"] is True
            assert planned.structured_content["data"]["preview"] == {
                "operations": [{"action": "upsert"}]
            }
            dry_args, dry_payload = gateway.calls[0]
            assert dry_args == (
                "tag",
                "replace-batch",
                "--file",
                "-",
                "--dry-run",
                "--json",
            )
            assert dry_payload == {
                "operations": [
                    {
                        "method": "GET",
                        "path": "/orders",
                        "tags": ["Orders"],
                        "folder": "EAM/Orders",
                    }
                ]
            }

            plan_id = planned.structured_content["data"]["plan_id"]
            applied = await client.call_tool("apifox_change_apply", {"plan_id": plan_id})
            assert applied.structured_content["ok"] is True
            apply_args, apply_payload = gateway.calls[1]
            assert apply_args == ("tag", "replace-batch", "--file", "-", "--json")
            assert apply_payload == dry_payload

    asyncio.run(scenario())


def test_batch_and_empty_folder_change_kinds_route_to_cli_stdin() -> None:
    async def scenario() -> None:
        gateway = FakeGateway()
        server = create_server(
            make_settings(), gateway=gateway, audit=AuditLogger(stream=io.StringIO())
        )
        async with Client(server) as client:
            cases = [
                (
                    "tags_replace_batch",
                    {"operations": [{"method": "GET", "path": "/a", "tags": []}]},
                    ("tag", "replace-batch"),
                ),
                (
                    "folder_move_batch",
                    {"operations": [{"method": "GET", "path": "/a", "folder_id": 12}]},
                    ("folder", "move-batch"),
                ),
                ("folder_delete_empty", {"all": True}, ("folder", "delete-empty")),
            ]
            for kind, spec, prefix in cases:
                planned = await client.call_tool(
                    "apifox_change_plan", {"kind": kind, "spec": spec}
                )
                assert planned.structured_content["ok"] is True
                args, payload = gateway.calls[-1]
                assert args[:2] == prefix
                assert args[-2:] == ("--dry-run", "--json")
                assert payload == spec

    asyncio.run(scenario())


def test_project_check_reports_actual_write_capabilities_and_redacts_nested_project_id() -> None:
    async def scenario() -> None:
        args = ("config", "check", "--json")
        gateway = ReadGateway(
            {args: {"configured": True, "connected": True, "project_id": "project-1234"}}
        )
        server = create_server(
            make_settings(write_mode=WriteMode.APPLY),
            gateway=gateway,
            audit=AuditLogger(stream=io.StringIO()),
        )
        async with Client(server) as client:
            result = await client.call_tool("apifox_project_check", {})
            data = result.structured_content["data"]
            assert data["project_id"] == "[REDACTED]"
            assert data["write_mode"] == "apply"
            assert data["capabilities"] == {"read": True, "plan": True, "apply": True}

    asyncio.run(scenario())


def test_api_get_normalizes_path_and_returns_failure_for_not_found() -> None:
    async def scenario() -> None:
        args = ("api", "get", "--method", "GET", "--path", "/missing", "--json")
        gateway = ReadGateway(
            {args: {"found": False, "method": "GET", "path": "/missing", "message": "missing"}}
        )
        server = create_server(
            make_settings(), gateway=gateway, audit=AuditLogger(stream=io.StringIO())
        )
        async with Client(server) as client:
            result = await client.call_tool(
                "apifox_api_get", {"method": "get", "path": "missing"}
            )
            assert result.structured_content["ok"] is False
            assert result.structured_content["error"]["code"] == "API_NOT_FOUND"
            assert gateway.calls[0][0] == args

    asyncio.run(scenario())
