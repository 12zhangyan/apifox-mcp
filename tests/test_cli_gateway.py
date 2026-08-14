from __future__ import annotations

import asyncio
import sys
from pathlib import Path

import pytest

from apifox_mcp.cli_gateway import CliGateway, redact_data, redact_text
from apifox_mcp.errors import GatewayError

from .conftest import make_settings


def write_fake_cli(path: Path) -> None:
    path.write_text(
        """
import json
import os
import sys
import time

if "read-session" in sys.argv:
    for line in sys.stdin:
        request = json.loads(line)
        print(json.dumps({
            "id": request["id"],
            "ok": True,
            "data": {"args": request["args"], "pid": os.getpid()},
        }), flush=True)
elif "--version" in sys.argv:
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
elif "--business-fail" in sys.argv:
    print(json.dumps({
        "kind": "endpoint",
        "import_result": {"success": False, "code": 403, "message": "import busy"},
    }))
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


def test_gateway_reuses_long_lived_read_session(tmp_path: Path) -> None:
    async def scenario() -> None:
        script = tmp_path / "fake_cli.py"
        write_fake_cli(script)
        gateway = CliGateway(make_settings(), command=[sys.executable, str(script)])
        try:
            first = await gateway.run_read(["api", "list", "--json"])
            second = await gateway.run_read(["schema", "list", "--json"])
        finally:
            await gateway.close()
        assert first.data["pid"] == second.data["pid"]
        assert first.data["args"] == ["api", "list"]
        assert second.data["args"] == ["schema", "list"]

    asyncio.run(scenario())


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


def test_gateway_rejects_business_failure_with_zero_exit_code(tmp_path: Path) -> None:
    script = tmp_path / "fake_cli.py"
    write_fake_cli(script)
    gateway = CliGateway(make_settings(), command=[sys.executable, str(script)])

    with pytest.raises(GatewayError) as caught:
        asyncio.run(gateway.run(["--business-fail"]))

    assert caught.value.code == "APIFOX_OPERATION_FAILED"
    assert caught.value.apifox_status == 403
    assert "import busy" in caught.value.message


def test_redact_text_handles_header_and_known_secret() -> None:
    output = redact_text(
        "Authorization: Bearer abc token=xyz cookie=session-value", ("session-value",)
    )
    assert "abc" not in output
    assert "xyz" not in output
    assert "session-value" not in output


def test_redact_data_masks_nested_examples_and_project_ids() -> None:
    payload = {
        "project_id": "project-1234",
        "parameters": [
            {"name": "Cookie", "example": "session=plain-cookie"},
            {"name": "Cookie", "examples": {"auth": {"value": "session=nested"}}},
        ],
        "requestBody": {
            "content": {
                "application/json": {
                    "schema": {
                        "properties": {
                            "password": {"type": "string", "example": "plain-password"},
                            "displayName": {"type": "string", "example": "Alice"},
                        }
                    },
                    "example": {"password": "plain-password", "displayName": "Alice"},
                }
            }
        },
        "responses": {
            "200": {
                "content": {
                    "application/json": {
                        "examples": {
                            "success": {
                                "value": '{"password":"json-password","displayName":"Alice"}'
                            }
                        }
                    }
                }
            }
        },
    }

    redacted = redact_data(payload, ("project-1234",))

    assert redacted["project_id"] == "[REDACTED]"
    assert redacted["parameters"][0]["example"] == "[REDACTED]"
    assert redacted["parameters"][1]["examples"] == "[REDACTED]"
    properties = redacted["requestBody"]["content"]["application/json"]["schema"]["properties"]
    assert properties["password"]["example"] == "[REDACTED]"
    assert properties["displayName"]["example"] == "Alice"
    body_example = redacted["requestBody"]["content"]["application/json"]["example"]
    assert body_example["password"] == "[REDACTED]"
    assert body_example["displayName"] == "Alice"
    response_value = redacted["responses"]["200"]["content"]["application/json"][
        "examples"
    ]["success"]["value"]
    assert "json-password" not in response_value
    assert '"password":"[REDACTED]"' in response_value
    assert '"displayName":"Alice"' in response_value
