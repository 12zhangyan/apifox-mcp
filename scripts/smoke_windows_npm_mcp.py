"""Start the frozen MCP through the npm launcher and verify tool discovery."""

from __future__ import annotations

import asyncio
import sys
from pathlib import Path

from mcp.client.session import ClientSession
from mcp.client.stdio import StdioServerParameters, stdio_client


async def _run(launcher: Path) -> None:
    params = StdioServerParameters(
        command="node",
        args=[str(launcher)],
        cwd=str(launcher.parents[2]),
    )
    async with stdio_client(params) as (read_stream, write_stream):
        async with ClientSession(read_stream, write_stream) as session:
            await session.initialize()
            tools = await session.list_tools()
            names = {tool.name for tool in tools.tools}
            required = {
                "apifox_project_overview",
                "apifox_change_plan",
                "apifox_change_apply",
            }
            missing = required - names
            if missing:
                raise RuntimeError(f"frozen npm MCP is missing tools: {sorted(missing)}")
            print(f"frozen npm MCP discovery passed with {len(names)} tools")


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: smoke_windows_npm_mcp.py PATH_TO_LAUNCHER", file=sys.stderr)
        return 2
    launcher = Path(sys.argv[1]).resolve()
    if not launcher.is_file():
        print(f"launcher not found: {launcher}", file=sys.stderr)
        return 2
    asyncio.run(_run(launcher))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
