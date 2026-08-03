from __future__ import annotations

import asyncio
from datetime import datetime, timedelta, timezone

import pytest

from apifox_mcp.errors import PolicyError
from apifox_mcp.models import ChangeKind
from apifox_mcp.plans import PlanStore
from apifox_mcp.policy import WritePolicy
from apifox_mcp.settings import WriteMode


def test_write_policy_defaults_are_enforced() -> None:
    with pytest.raises(PolicyError, match="disabled"):
        WritePolicy(WriteMode.DISABLED).require_plan()
    with pytest.raises(PolicyError, match="APIFOX_MCP_WRITE_MODE=apply"):
        WritePolicy(WriteMode.PLAN).require_apply()
    WritePolicy(WriteMode.APPLY).require_plan()
    WritePolicy(WriteMode.APPLY).require_apply()


def test_plan_is_bound_to_project_version_and_single_use() -> None:
    async def scenario() -> None:
        store = PlanStore(60)
        record = await store.create(
            kind=ChangeKind.ENDPOINT_UPSERT,
            args=["api", "upsert", "--file", "-", "--json"],
            stdin_payload={"method": "POST", "path": "/orders"},
            preview={"command": "api upsert"},
            project_id="p1",
            cli_version="v1",
        )
        acquired = await store.acquire(record.plan_id, project_id="p1", cli_version="v1")
        assert acquired.payload_sha256 == record.payload_sha256
        await store.finish(record.plan_id, success=True)
        with pytest.raises(PolicyError) as caught:
            await store.acquire(record.plan_id, project_id="p1", cli_version="v1")
        assert caught.value.code == "PLAN_CONSUMED"

    asyncio.run(scenario())


def test_plan_rejects_tampering_and_expiry() -> None:
    async def scenario() -> None:
        store = PlanStore(60)
        tampered = await store.create(
            kind=ChangeKind.SCHEMA_CREATE,
            args=["schema", "create"],
            stdin_payload={"name": "Order"},
            preview={},
            project_id="p1",
            cli_version="v1",
        )
        tampered.stdin_payload = {"name": "Changed"}
        with pytest.raises(PolicyError) as caught:
            await store.acquire(tampered.plan_id, project_id="p1", cli_version="v1")
        assert caught.value.code == "PLAN_TAMPERED"

        expired = await store.create(
            kind=ChangeKind.SCHEMA_CREATE,
            args=["schema", "create"],
            stdin_payload={"name": "Order"},
            preview={},
            project_id="p1",
            cli_version="v1",
        )
        expired.expires_at = datetime.now(timezone.utc) - timedelta(seconds=1)
        with pytest.raises(PolicyError) as caught:
            await store.acquire(expired.plan_id, project_id="p1", cli_version="v1")
        assert caught.value.code == "PLAN_EXPIRED"

    asyncio.run(scenario())
