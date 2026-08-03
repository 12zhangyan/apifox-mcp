"""Redacted JSONL audit events for MCP tool calls."""

from __future__ import annotations

import asyncio
import json
import sys
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import TextIO

from .errors import McpServiceError


@dataclass(frozen=True, slots=True)
class AuditEvent:
    request_id: str
    tool: str
    mode: str
    project_id: str
    outcome: str
    duration_ms: int
    error_code: str | None = None
    plan_id: str | None = None
    payload_sha256: str | None = None
    cli_exit_code: int | None = None


class AuditLogger:
    def __init__(self, path: Path | None = None, *, stream: TextIO | None = None) -> None:
        self.path = path
        self.stream = stream or sys.stderr
        self._lock = asyncio.Lock()
        self._file: TextIO | None = None
        if path is not None:
            try:
                path.parent.mkdir(parents=True, exist_ok=True)
                self._file = path.open("a", encoding="utf-8")
            except OSError as exc:
                raise McpServiceError("AUDIT_UNAVAILABLE", f"cannot open audit log: {exc}") from exc

    def ensure_write_ready(self) -> None:
        if self.path is not None and (self._file is None or self._file.closed):
            raise McpServiceError(
                "AUDIT_UNAVAILABLE",
                "audit log is unavailable; refusing external write",
            )

    async def record(self, event: AuditEvent) -> None:
        payload = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            **asdict(event),
        }
        line = json.dumps(payload, ensure_ascii=False, separators=(",", ":")) + "\n"
        async with self._lock:
            try:
                target = self._file or self.stream
                target.write(line)
                target.flush()
            except OSError as exc:
                raise McpServiceError(
                    "AUDIT_UNAVAILABLE", f"cannot write audit event: {exc}"
                ) from exc

    async def close(self) -> None:
        async with self._lock:
            if self._file is not None and not self._file.closed:
                self._file.close()
