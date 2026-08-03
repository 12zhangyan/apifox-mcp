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
