from __future__ import annotations

from dataclasses import replace

import pytest

from apifox_mcp.settings import Settings, SettingsError, Transport, _bundled_cli_path

from .conftest import make_settings


def test_non_loopback_http_requires_auth_and_origin_controls() -> None:
    settings = replace(
        make_settings(),
        transport=Transport.STREAMABLE_HTTP,
        host="0.0.0.0",
        http_bearer_token=None,
        auth_issuer_url=None,
        resource_server_url=None,
        allowed_hosts=(),
        allowed_origins=(),
    )
    with pytest.raises(SettingsError, match="HTTP_BEARER_TOKEN"):
        settings.validate_transport()


def test_loopback_http_is_allowed_without_auth() -> None:
    settings = replace(make_settings(), transport=Transport.STREAMABLE_HTTP, host="127.0.0.1")
    settings.validate_transport()


def test_bundled_cli_path_uses_platform_binary(tmp_path) -> None:
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    windows_cli = bin_dir / "apifox-cli.exe"
    windows_cli.write_bytes(b"test")

    assert _bundled_cli_path(tmp_path, windows=True) == str(windows_cli)
    assert _bundled_cli_path(tmp_path, windows=False) is None

    unix_cli = bin_dir / "apifox-cli"
    unix_cli.write_bytes(b"test")
    assert _bundled_cli_path(tmp_path, windows=False) == str(unix_cli)


def test_default_cli_timeout_allows_metadata_and_import_writes(monkeypatch) -> None:
    monkeypatch.delenv("APIFOX_MCP_CLI_TIMEOUT", raising=False)
    assert Settings.from_env().cli_timeout_seconds == 120
