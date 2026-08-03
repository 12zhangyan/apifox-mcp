"""Console entry point used only for the standalone Windows MCP executable."""

from __future__ import annotations

from multiprocessing import freeze_support

from apifox_mcp.main import main

if __name__ == "__main__":
    freeze_support()
    main()
