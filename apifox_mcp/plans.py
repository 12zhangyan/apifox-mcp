"""Short-lived, single-use plans that bind previews to exact CLI inputs."""

from __future__ import annotations

import asyncio
import hashlib
import json
import secrets
from collections.abc import Sequence
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any

from .errors import PolicyError
from .models import ChangeKind


@dataclass(slots=True)
class PlanRecord:
    plan_id: str
    kind: ChangeKind
    args: tuple[str, ...]
    stdin_payload: dict[str, Any] | None
    preview: dict[str, Any]
    payload_sha256: str
    project_id: str
    cli_version: str | None
    created_at: datetime
    expires_at: datetime
    state: str = "pending"


class PlanStore:
    def __init__(self, ttl_seconds: int) -> None:
        self._ttl = timedelta(seconds=ttl_seconds)
        self._plans: dict[str, PlanRecord] = {}
        self._lock = asyncio.Lock()

    @staticmethod
    def _digest(
        kind: ChangeKind,
        args: Sequence[str],
        stdin_payload: dict[str, Any] | None,
        project_id: str,
        cli_version: str | None,
    ) -> str:
        canonical = json.dumps(
            {
                "kind": kind.value,
                "args": list(args),
                "stdin": stdin_payload,
                "project_id": project_id,
                "cli_version": cli_version,
            },
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        ).encode("utf-8")
        return hashlib.sha256(canonical).hexdigest()

    async def create(
        self,
        *,
        kind: ChangeKind,
        args: Sequence[str],
        stdin_payload: dict[str, Any] | None,
        preview: dict[str, Any],
        project_id: str,
        cli_version: str | None,
    ) -> PlanRecord:
        now = datetime.now(timezone.utc)
        record = PlanRecord(
            plan_id="plan_" + secrets.token_urlsafe(18),
            kind=kind,
            args=tuple(args),
            stdin_payload=stdin_payload,
            preview=preview,
            payload_sha256=self._digest(kind, args, stdin_payload, project_id, cli_version),
            project_id=project_id,
            cli_version=cli_version,
            created_at=now,
            expires_at=now + self._ttl,
        )
        async with self._lock:
            self._plans[record.plan_id] = record
            self._purge_expired(now)
        return record

    def _purge_expired(self, now: datetime) -> None:
        expired = [
            plan_id
            for plan_id, record in self._plans.items()
            if record.expires_at <= now and record.state != "in_progress"
        ]
        for plan_id in expired:
            del self._plans[plan_id]

    async def acquire(
        self,
        plan_id: str,
        *,
        project_id: str,
        cli_version: str | None,
    ) -> PlanRecord:
        async with self._lock:
            record = self._plans.get(plan_id)
            if record is None:
                raise PolicyError("PLAN_NOT_FOUND", "plan does not exist or has expired")
            if record.expires_at <= datetime.now(timezone.utc):
                del self._plans[plan_id]
                raise PolicyError("PLAN_EXPIRED", "plan has expired")
            if record.project_id != project_id:
                raise PolicyError("PLAN_PROJECT_MISMATCH", "plan belongs to another project")
            if record.cli_version != cli_version:
                raise PolicyError("PLAN_CLI_CHANGED", "apifox-cli version changed after planning")
            if record.state == "in_progress":
                raise PolicyError("PLAN_BUSY", "plan is already being applied", retryable=True)
            if record.state == "consumed":
                raise PolicyError("PLAN_CONSUMED", "plan has already been applied")
            digest = self._digest(
                record.kind,
                record.args,
                record.stdin_payload,
                record.project_id,
                record.cli_version,
            )
            if not secrets.compare_digest(digest, record.payload_sha256):
                raise PolicyError("PLAN_TAMPERED", "plan payload integrity check failed")
            record.state = "in_progress"
            return record

    async def finish(self, plan_id: str, *, success: bool, retryable: bool = False) -> None:
        async with self._lock:
            record = self._plans.get(plan_id)
            if record is None:
                return
            record.state = "consumed" if success or not retryable else "pending"
