"""Hatch build hook for platform wheels that bundle the Go CLI."""

from __future__ import annotations

import os
from pathlib import Path

from hatchling.builders.hooks.plugin.interface import BuildHookInterface


class CustomBuildHook(BuildHookInterface):
    """Inject a prebuilt CLI and mark the wheel as platform-specific."""

    PLUGIN_NAME = "custom"

    def initialize(self, version: str, build_data: dict) -> None:
        if self.target_name != "wheel":
            return

        raw_binary = os.getenv("APIFOX_BUNDLED_CLI")
        if not raw_binary:
            return

        binary = Path(raw_binary).expanduser().resolve()
        if not binary.is_file():
            raise RuntimeError(f"APIFOX_BUNDLED_CLI does not exist: {binary}")

        wheel_tag = os.getenv("APIFOX_WHEEL_TAG", "").strip()
        if not wheel_tag or wheel_tag == "py3-none-any":
            raise RuntimeError(
                "APIFOX_WHEEL_TAG must be a platform-specific wheel tag when bundling the CLI"
            )

        target_name = "apifox-cli.exe" if binary.suffix.lower() == ".exe" else "apifox-cli"
        target = f"apifox_mcp/bin/{target_name}"
        build_data.setdefault("force_include", {})[str(binary)] = target
        build_data["tag"] = wheel_tag
        build_data["pure_python"] = False
