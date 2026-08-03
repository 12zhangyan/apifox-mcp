from __future__ import annotations

from dataclasses import replace
from typing import Any

from apifox_mcp.settings import Settings, WriteMode


def make_settings(**overrides: Any) -> Settings:
    defaults = Settings.from_env()
    configured = replace(
        defaults,
        token="test-token-secret",
        project_id="project-1234",
        cli_path="apifox-cli",
        write_mode=WriteMode.PLAN,
        audit_log_path=None,
        cli_timeout_seconds=2,
        max_input_bytes=4096,
        max_output_bytes=4096,
        max_concurrency=2,
        plan_ttl_seconds=60,
    )
    return replace(configured, **overrides)
