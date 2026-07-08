"""
Apifox MCP 服务器包
==================

提供与 Apifox API 交互的 MCP 工具集。
"""

__version__ = "0.1.0"
__all__ = ["mcp", "logger", "__version__"]


def __getattr__(name: str):
    """Lazy-load MCP objects until the Python server needs them."""
    if name in {"mcp", "logger"}:
        from .config import logger, mcp

        return {"mcp": mcp, "logger": logger}[name]
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
