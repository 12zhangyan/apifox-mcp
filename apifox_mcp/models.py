"""Pydantic contracts returned as MCP structured content."""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


class ChangeKind(str, Enum):
    ENDPOINT_CREATE = "endpoint_create"
    ENDPOINT_UPDATE = "endpoint_update"
    ENDPOINT_UPSERT = "endpoint_upsert"
    SCHEMA_CREATE = "schema_create"
    SCHEMA_UPDATE = "schema_update"
    APPLY_DOCS = "apply_docs"
    GENERATE_CRUD = "generate_crud"
    TAGS_REPLACE = "tags_replace"


class ErrorInfo(BaseModel):
    code: str
    message: str
    retryable: bool = False
    exit_code: int | None = None
    apifox_status: int | None = None
    details: dict[str, Any] | None = None


class ResultMeta(BaseModel):
    duration_ms: int = 0
    cli_version: str | None = None


class ToolResult(BaseModel):
    ok: bool
    request_id: str
    tool: str
    project_id: str
    mode: str
    data: dict[str, Any] = Field(default_factory=dict)
    error: ErrorInfo | None = None
    meta: ResultMeta = Field(default_factory=ResultMeta)


class ChangePlanData(BaseModel):
    plan_id: str
    kind: ChangeKind
    payload_sha256: str
    expires_at: str
    preview: dict[str, Any]
