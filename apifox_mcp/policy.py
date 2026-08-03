"""Server-side authorization for planning and applying Apifox changes."""

from __future__ import annotations

from .errors import PolicyError
from .settings import WriteMode


class WritePolicy:
    def __init__(self, mode: WriteMode) -> None:
        self.mode = mode

    def require_plan(self) -> None:
        if self.mode is WriteMode.DISABLED:
            raise PolicyError(
                "WRITE_DISABLED",
                "change planning is disabled by APIFOX_MCP_WRITE_MODE",
            )

    def require_apply(self) -> None:
        if self.mode is not WriteMode.APPLY:
            raise PolicyError(
                "APPLY_DISABLED",
                "real writes require APIFOX_MCP_WRITE_MODE=apply",
            )
