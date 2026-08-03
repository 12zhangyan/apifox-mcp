from __future__ import annotations

import asyncio
import sys
from pathlib import Path

import pytest

from apifox_mcp.cli_gateway import CliGateway, redact_text
from apifox_mcp.errors import GatewayError

from .conftest import make_settings


def write_fake_cli(path: Path) -> None:
    path.write_text(
        """
import json
import os
import sys
import time

if "--version" in sys.argv:
    print("apifox-cli test-version")
elif "--fail" in sys.argv:
    print("Authorization: Bearer " + os.environ["APIFOX_TOKEN"], file=sys.stderr)
    raise SystemExit(1)
elif "--structured-fail" in sys.argv:
    print(json.dumps({
        "configured": False,
        "connected": False,
        "error": {"code": "MISSING_CREDENTIALS", "message": "credentials missing"},
    }))
    raise SystemExit(2)
elif "--validation-fail" in sys.argv:
    print("spec validation failed: missing description", file=sys.stderr)
    raise SystemExit(2)
elif "--sleep" in sys.argv:
    time.sleep(5)
else:
    raw = sys.stdin.read()
    print(json.dumps({"argv": sys.argv[1:], "stdin": json.loads(raw) if raw else None}))
""".strip(),
        encoding="utf-8",
    )


def test_gateway_uses_stdin_and_never_places_token_in_argv(tmp_path: Path) -> None:
    script = tmp_path / "fake_cli.py"
    write_fake_cli(script)
    settings = make_settings()
    gateway = CliGateway(settings, command=[sys.executable, str(script)])

    response = asyncio.run(
        gateway.run(["api", "upsert", "--file", "-", "--dry-run"], stdin_payload={"x": 1})
    )

    assert response.data["stdin"] == {"x": 1}
    assert settings.token not in " ".join(response.data["argv"])


def test_gateway_redacts_cli_errors(tmp_path: Path) -> None:
    script = tmp_path / "fake_cli.py"
    write_fake_cli(script)
    settings = make_settings()
    gateway = CliGateway(settings, command=[sys.executable, str(script)])

    with pytest.raises(GatewayError) as caught:
        asyncio.run(gateway.run(["--fail"]))

    assert caught.value.code == "CLI_FAILED"
    assert settings.token not in caught.value.message
    assert "[REDACTED]" in caught.value.message


def test_gateway_preserves_structured_cli_error(tmp_path: Path) -> None:
    script = tmp_path / "fake_cli.py"
    write_fake_cli(script)
    gateway = CliGateway(make_settings(), command=[sys.executable, str(script)])

    with pytest.raises(GatewayError) as caught:
        asyncio.run(gateway.run(["--structured-fail"]))

    assert caught.value.code == "MISSING_CREDENTIALS"
    assert caught.value.exit_code == 2
    assert caught.value.details == {"configured": False, "connected": False}


def test_gateway_classifies_validation_failure(tmp_path: Path) -> None:
    script = tmp_path / "fake_cli.py"
    write_fake_cli(script)
    gateway = CliGateway(make_settings(), command=[sys.executable, str(script)])

    with pytest.raises(GatewayError) as caught:
        asyncio.run(gateway.run(["--validation-fail"]))

    assert caught.value.code == "VALIDATION_FAILED"


def test_gateway_enforces_timeout(tmp_path: Path) -> None:
    script = tmp_path / "fake_cli.py"
    write_fake_cli(script)
    gateway = CliGateway(
        make_settings(cli_timeout_seconds=1), command=[sys.executable, str(script)]
    )

    with pytest.raises(GatewayError) as caught:
        asyncio.run(gateway.run(["--sleep"]))

    assert caught.value.code == "CLI_TIMEOUT"
    assert caught.value.retryable is True


def test_redact_text_handles_header_and_known_secret() -> None:
    output = redact_text(
        "Authorization: Bearer abc token=xyz cookie=session-value", ("session-value",)
    )
    assert "abc" not in output
    assert "xyz" not in output
    assert "session-value" not in output
