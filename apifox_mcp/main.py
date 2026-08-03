"""Command line entry point for the enterprise Apifox MCP server."""

from __future__ import annotations

import argparse
import sys

from mcp.server.transport_security import TransportSecuritySettings

from .server import create_server
from .settings import Settings, SettingsError, Transport


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Run the Apifox Enterprise MCP server")
    parser.add_argument("--transport", choices=[item.value for item in Transport])
    parser.add_argument("--host")
    parser.add_argument("--port", type=int)
    parser.add_argument("--path")
    return parser


def main() -> None:
    """Run stdio by default, or a guarded Streamable HTTP server when configured."""
    args = build_parser().parse_args()
    try:
        settings = Settings.from_env()
        if any(value is not None for value in (args.transport, args.host, args.port, args.path)):
            from dataclasses import replace

            settings = replace(
                settings,
                transport=Transport(args.transport) if args.transport else settings.transport,
                host=args.host or settings.host,
                port=args.port or settings.port,
                http_path=args.path or settings.http_path,
            )
        settings.validate_transport()
    except SettingsError as exc:
        print(f"configuration error: {exc}", file=sys.stderr)
        raise SystemExit(2) from exc

    server = create_server(settings)
    if settings.transport is Transport.STDIO:
        server.run("stdio")
        return

    allowed_hosts = list(settings.allowed_hosts)
    allowed_origins = list(settings.allowed_origins)
    if not allowed_hosts:
        allowed_hosts = ["127.0.0.1:*", "localhost:*", "[::1]:*"]
    if not allowed_origins:
        allowed_origins = [
            "http://127.0.0.1:*",
            "http://localhost:*",
            "http://[::1]:*",
        ]
    security = TransportSecuritySettings(
        enable_dns_rebinding_protection=True,
        allowed_hosts=allowed_hosts,
        allowed_origins=allowed_origins,
    )
    server.run(
        "streamable-http",
        host=settings.host,
        port=settings.port,
        streamable_http_path=settings.http_path,
        transport_security=security,
    )


if __name__ == "__main__":
    main()
