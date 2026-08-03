from __future__ import annotations

import json
from pathlib import Path

import tomllib

ROOT = Path(__file__).resolve().parents[1]


def test_npm_and_python_versions_match() -> None:
    python_project = tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))
    npm_project = json.loads((ROOT / "npm/package.json").read_text(encoding="utf-8"))

    assert npm_project["version"] == python_project["project"]["version"]


def test_npm_package_is_explicitly_windows_x64() -> None:
    npm_project = json.loads((ROOT / "npm/package.json").read_text(encoding="utf-8"))

    assert npm_project["os"] == ["win32"]
    assert npm_project["cpu"] == ["x64"]
    assert npm_project["bin"] == {
        "apifox-mcp": "bin/apifox-mcp.cjs",
        "apifox-cli": "bin/apifox-cli.cjs",
    }
