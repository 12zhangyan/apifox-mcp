"""Verify that a release wheel contains one correctly tagged Go CLI."""

from __future__ import annotations

import argparse
import zipfile
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("wheel", type=Path)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--binary", required=True)
    args = parser.parse_args()

    expected_suffix = f"-{args.tag}.whl"
    if not args.wheel.name.endswith(expected_suffix):
        raise SystemExit(f"wheel filename does not end with {expected_suffix}: {args.wheel.name}")

    expected_binary = f"apifox_mcp/bin/{args.binary}"
    with zipfile.ZipFile(args.wheel) as archive:
        names = archive.namelist()
        wheel_metadata = [name for name in names if name.endswith(".dist-info/WHEEL")]
        if len(wheel_metadata) != 1:
            raise SystemExit(f"expected one WHEEL metadata file, found {wheel_metadata}")
        metadata = archive.read(wheel_metadata[0]).decode("utf-8")
        if f"Tag: {args.tag}" not in metadata:
            raise SystemExit(f"WHEEL metadata is missing Tag: {args.tag}")
        if expected_binary not in names:
            raise SystemExit(f"wheel is missing {expected_binary}")
        binaries = [name for name in names if name.startswith("apifox_mcp/bin/")]
        if binaries != [expected_binary]:
            raise SystemExit(f"wheel contains unexpected CLI files: {binaries}")
        binary_info = archive.getinfo(expected_binary)
        if binary_info.file_size == 0:
            raise SystemExit("bundled CLI is empty")
        if not args.binary.endswith(".exe"):
            unix_mode = binary_info.external_attr >> 16
            if unix_mode & 0o111 == 0:
                raise SystemExit("bundled Unix CLI is not executable")

    print(f"verified {args.wheel.name}: {expected_binary}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
