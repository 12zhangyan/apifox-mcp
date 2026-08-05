"""Runtime settings for the enterprise MCP server."""

from __future__ import annotations

import os
import shutil
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from urllib.parse import urlparse


class SettingsError(ValueError):
    """Raised when runtime settings would create an unsafe server."""


class WriteMode(str, Enum):
    DISABLED = "disabled"
    PLAN = "plan"
    APPLY = "apply"


class Transport(str, Enum):
    STDIO = "stdio"
    STREAMABLE_HTTP = "streamable-http"


def _env_int(name: str, default: int, *, minimum: int = 1) -> int:
    raw = os.getenv(name)
    if raw is None:
        return default
    try:
        value = int(raw)
    except ValueError as exc:
        raise SettingsError(f"{name} must be an integer") from exc
    if value < minimum:
        raise SettingsError(f"{name} must be >= {minimum}")
    return value


def _env_list(name: str) -> tuple[str, ...]:
    return tuple(value.strip() for value in os.getenv(name, "").split(",") if value.strip())


def _is_loopback(host: str) -> bool:
    return host.lower() in {"127.0.0.1", "localhost", "::1"}


def _bundled_cli_path(
    package_dir: Path | None = None,
    *,
    windows: bool | None = None,
) -> str | None:
    """Return the CLI shipped in a platform wheel, if present."""
    root = package_dir or Path(__file__).resolve().parent
    is_windows = os.name == "nt" if windows is None else windows
    names = ("apifox-cli.exe", "apifox-cli") if is_windows else ("apifox-cli",)
    for name in names:
        candidate = root / "bin" / name
        if candidate.is_file():
            return str(candidate)
    return None


def _resolve_cli_path() -> str:
    return (
        os.getenv("APIFOX_CLI_PATH")
        or _bundled_cli_path()
        or shutil.which("apifox-cli")
        or "apifox-cli"
    )


@dataclass(frozen=True, slots=True)
class Settings:
    token: str | None
    project_id: str | None
    base_url: str
    cli_path: str
    write_mode: WriteMode
    cli_timeout_seconds: int
    max_input_bytes: int
    max_output_bytes: int
    max_concurrency: int
    plan_ttl_seconds: int
    audit_log_path: Path | None
    transport: Transport
    host: str
    port: int
    http_path: str
    allowed_hosts: tuple[str, ...]
    allowed_origins: tuple[str, ...]
    http_bearer_token: str | None
    auth_issuer_url: str | None
    resource_server_url: str | None
    auth_scopes: tuple[str, ...]

    @classmethod
    def from_env(cls) -> Settings:
        try:
            write_mode = WriteMode(os.getenv("APIFOX_MCP_WRITE_MODE", "plan").lower())
        except ValueError as exc:
            raise SettingsError("APIFOX_MCP_WRITE_MODE must be disabled, plan, or apply") from exc
        try:
            transport = Transport(os.getenv("APIFOX_MCP_TRANSPORT", "stdio").lower())
        except ValueError as exc:
            raise SettingsError("APIFOX_MCP_TRANSPORT must be stdio or streamable-http") from exc

        base_url = os.getenv("APIFOX_BASE_URL", "https://api.apifox.com").rstrip("/")
        parsed = urlparse(base_url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise SettingsError("APIFOX_BASE_URL must be an absolute HTTP(S) URL")
        if parsed.scheme == "http" and parsed.hostname not in {"127.0.0.1", "localhost", "::1"}:
            raise SettingsError("APIFOX_BASE_URL must use HTTPS unless it targets loopback")

        audit_path = os.getenv("APIFOX_MCP_AUDIT_LOG")
        cli_path = _resolve_cli_path()
        return cls(
            token=os.getenv("APIFOX_TOKEN"),
            project_id=os.getenv("APIFOX_PROJECT_ID"),
            base_url=base_url,
            cli_path=cli_path,
            write_mode=write_mode,
            cli_timeout_seconds=_env_int("APIFOX_MCP_CLI_TIMEOUT", 120),
            max_input_bytes=_env_int("APIFOX_MCP_MAX_INPUT_BYTES", 2 * 1024 * 1024),
            max_output_bytes=_env_int("APIFOX_MCP_MAX_OUTPUT_BYTES", 8 * 1024 * 1024),
            max_concurrency=_env_int("APIFOX_MCP_MAX_CONCURRENCY", 4),
            plan_ttl_seconds=_env_int("APIFOX_MCP_PLAN_TTL", 600),
            audit_log_path=Path(audit_path).expanduser() if audit_path else None,
            transport=transport,
            host=os.getenv("APIFOX_MCP_HOST", "127.0.0.1"),
            port=_env_int("APIFOX_MCP_PORT", 8000),
            http_path=os.getenv("APIFOX_MCP_HTTP_PATH", "/mcp"),
            allowed_hosts=_env_list("APIFOX_MCP_ALLOWED_HOSTS"),
            allowed_origins=_env_list("APIFOX_MCP_ALLOWED_ORIGINS"),
            http_bearer_token=os.getenv("APIFOX_MCP_HTTP_BEARER_TOKEN"),
            auth_issuer_url=os.getenv("APIFOX_MCP_AUTH_ISSUER_URL"),
            resource_server_url=os.getenv("APIFOX_MCP_RESOURCE_SERVER_URL"),
            auth_scopes=_env_list("APIFOX_MCP_AUTH_SCOPES") or ("apifox.read",),
        )

    def validate_transport(self) -> None:
        if self.transport is Transport.STDIO:
            return
        if not self.http_path.startswith("/"):
            raise SettingsError("APIFOX_MCP_HTTP_PATH must start with '/'")
        if _is_loopback(self.host):
            return
        missing = []
        if not self.http_bearer_token:
            missing.append("APIFOX_MCP_HTTP_BEARER_TOKEN")
        if not self.auth_issuer_url:
            missing.append("APIFOX_MCP_AUTH_ISSUER_URL")
        if not self.resource_server_url:
            missing.append("APIFOX_MCP_RESOURCE_SERVER_URL")
        if not self.allowed_hosts:
            missing.append("APIFOX_MCP_ALLOWED_HOSTS")
        if not self.allowed_origins:
            missing.append("APIFOX_MCP_ALLOWED_ORIGINS")
        if missing:
            raise SettingsError("non-loopback Streamable HTTP requires: " + ", ".join(missing))

    @property
    def project_label(self) -> str:
        if not self.project_id:
            return "unconfigured"
        if len(self.project_id) <= 4:
            return "***"
        return "***" + self.project_id[-4:]
