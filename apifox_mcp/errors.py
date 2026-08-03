"""Stable error types shared by MCP tools."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any


@dataclass(slots=True)
class McpServiceError(Exception):
    code: str
    message: str
    retryable: bool = False
    exit_code: int | None = None
    apifox_status: int | None = None
    details: dict[str, Any] | None = None

    def __str__(self) -> str:
        return self.message


class GatewayError(McpServiceError):
    """A failure while invoking or decoding the Go CLI."""


class PolicyError(McpServiceError):
    """A write policy or plan lifecycle failure."""
