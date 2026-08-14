"""Safe asynchronous adapter around the Go ``apifox-cli`` binary."""

from __future__ import annotations

import asyncio
import json
import os
import re
import time
import uuid
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Any

from .errors import GatewayError
from .settings import Settings

_SAFE_ENV_KEYS = (
    "PATH",
    "SYSTEMROOT",
    "COMSPEC",
    "PATHEXT",
    "TEMP",
    "TMP",
    "HOME",
    "USERPROFILE",
    "LANG",
    "LC_ALL",
    "SSL_CERT_FILE",
    "SSL_CERT_DIR",
)
_HTTP_STATUS_RE = re.compile(r"\bHTTP\s+(\d{3})\b", re.IGNORECASE)
_ERROR_CODE_RE = re.compile(r"^[A-Z][A-Z0-9_]{0,63}$")
_VALIDATION_FAILURE_RE = re.compile(r"(?:校验失败|validation failed)", re.IGNORECASE)
_SECRET_RE = re.compile(
    r"(?i)(authorization\s*[:=]\s*bearer\s+|bearer\s+|token\s*[:=]\s*|cookie\s*[:=]\s*)[^\s,;]+"
)
_SENSITIVE_KEY_RE = re.compile(
    r"(?i)(?:password|passwd|pwd|secret|token|authorization|cookie|api[-_]?key|session|credential|project[-_]?id)"
)
_SENSITIVE_VALUE_KEYS = {"example", "examples", "default", "value", "enum", "const"}


@dataclass(frozen=True, slots=True)
class GatewayResponse:
    data: dict[str, Any]
    duration_ms: int


def redact_text(value: str, secrets: Sequence[str | None] = ()) -> str:
    redacted = value
    for secret in secrets:
        if secret:
            redacted = redacted.replace(secret, "[REDACTED]")
    return _SECRET_RE.sub(lambda match: match.group(1) + "[REDACTED]", redacted)


def redact_data(value: Any, secrets: Sequence[str | None] = (), *, sensitive: bool = False) -> Any:
    """Recursively redact credentials and examples while retaining schema structure."""
    if isinstance(value, dict):
        named_sensitive = _SENSITIVE_KEY_RE.search(str(value.get("name", ""))) is not None
        result: dict[str, Any] = {}
        for raw_key, child in value.items():
            key = str(raw_key)
            key_sensitive = _SENSITIVE_KEY_RE.search(key) is not None
            redact_leaf = (sensitive or named_sensitive) and key.lower() in _SENSITIVE_VALUE_KEYS
            if redact_leaf:
                result[key] = "[REDACTED]"
            elif key_sensitive and not isinstance(child, (dict, list)):
                result[key] = "[REDACTED]"
            else:
                result[key] = redact_data(
                    child,
                    secrets,
                    sensitive=sensitive or named_sensitive or key_sensitive,
                )
        return result
    if isinstance(value, list):
        return [redact_data(child, secrets, sensitive=sensitive) for child in value]
    if isinstance(value, str):
        redacted = value
        for secret in secrets:
            if secret:
                redacted = redacted.replace(secret, "[REDACTED]")
        if redacted.lstrip().startswith(("{", "[")):
            try:
                decoded = json.loads(redacted)
            except json.JSONDecodeError:
                pass
            else:
                return json.dumps(
                    redact_data(decoded, secrets, sensitive=sensitive),
                    ensure_ascii=False,
                    separators=(",", ":"),
                )
        return redact_text(redacted)
    return value


def _business_failure(payload: dict[str, Any]) -> tuple[str, int | None] | None:
    candidates = [payload]
    import_result = payload.get("import_result")
    if isinstance(import_result, dict):
        candidates.append(import_result)
    for candidate in candidates:
        failed = candidate.get("success") is False or candidate.get("ok") is False
        status: int | None = None
        for key in ("status", "statusCode", "code"):
            value = candidate.get(key)
            if isinstance(value, int) and 400 <= value <= 599:
                status = value
                failed = True
                break
        raw_error = candidate.get("error")
        if isinstance(raw_error, dict) and raw_error:
            failed = True
        if not failed:
            continue
        message = candidate.get("message") or candidate.get("errorMessage")
        if not isinstance(message, str) or not message.strip():
            if isinstance(raw_error, dict):
                raw_message = raw_error.get("message")
                message = raw_message if isinstance(raw_message, str) else None
        return (message or "Apifox returned an unsuccessful result", status)
    return None


class CliGateway:
    def __init__(
        self,
        settings: Settings,
        *,
        command: Sequence[str] | None = None,
    ) -> None:
        self.settings = settings
        self.command = tuple(command or (settings.cli_path,))
        self._semaphore = asyncio.Semaphore(settings.max_concurrency)
        self._version: str | None = None
        self._read_process: asyncio.subprocess.Process | None = None
        self._read_lock = asyncio.Lock()

    def _environment(self) -> dict[str, str]:
        env = {key: value for key in _SAFE_ENV_KEYS if (value := os.getenv(key)) is not None}
        if self.settings.token:
            env["APIFOX_TOKEN"] = self.settings.token
        if self.settings.project_id:
            env["APIFOX_PROJECT_ID"] = self.settings.project_id
        env["APIFOX_BASE_URL"] = self.settings.base_url
        env["APIFOX_MCP_READ_CACHE_TTL"] = str(self.settings.read_cache_ttl_seconds)
        return env

    async def version(self) -> str | None:
        if self._version is not None:
            return self._version
        try:
            response = await self._invoke(("--version",), None, expect_json=False)
        except GatewayError:
            return None
        version = str(response.data.get("text", "")).strip()
        self._version = version or None
        return self._version

    async def run(
        self,
        args: Sequence[str],
        *,
        stdin_payload: dict[str, Any] | None = None,
        expect_json: bool = True,
    ) -> GatewayResponse:
        return await self._invoke(tuple(args), stdin_payload, expect_json=expect_json)

    async def run_read(self, args: Sequence[str]) -> GatewayResponse:
        started = time.perf_counter()
        async with self._read_lock:
            process = await self._ensure_read_process()
            request_id = "read_" + uuid.uuid4().hex
            request = json.dumps(
                {"id": request_id, "args": [arg for arg in args if arg != "--json"]},
                ensure_ascii=False,
                separators=(",", ":"),
            ).encode("utf-8") + b"\n"
            assert process.stdin is not None
            assert process.stdout is not None
            try:
                process.stdin.write(request)
                await process.stdin.drain()
                line = await asyncio.wait_for(
                    process.stdout.readline(), timeout=self.settings.cli_timeout_seconds
                )
            except TimeoutError as exc:
                await self._stop_read_process()
                raise GatewayError(
                    "CLI_TIMEOUT",
                    f"apifox-cli read session exceeded {self.settings.cli_timeout_seconds} seconds",
                    retryable=True,
                ) from exc
            except (BrokenPipeError, ConnectionError) as exc:
                await self._stop_read_process()
                raise GatewayError(
                    "CLI_FAILED", "apifox-cli read session stopped", retryable=True
                ) from exc
            if not line:
                await self._stop_read_process()
                raise GatewayError(
                    "CLI_FAILED",
                    "apifox-cli read session returned no response",
                    retryable=True,
                )
            if len(line) > self.settings.max_output_bytes:
                raise GatewayError(
                    "CLI_OUTPUT_TOO_LARGE",
                    f"apifox-cli output exceeds {self.settings.max_output_bytes} bytes",
                )
            try:
                response = json.loads(line)
            except json.JSONDecodeError as exc:
                raise GatewayError(
                    "INVALID_CLI_OUTPUT", "apifox-cli read session returned invalid JSON"
                ) from exc
            if response.get("id") != request_id:
                raise GatewayError(
                    "INVALID_CLI_OUTPUT", "apifox-cli read session response ID mismatch"
                )
            if response.get("ok") is not True:
                raw_error = response.get("error") if isinstance(response.get("error"), dict) else {}
                raise GatewayError(
                    str(raw_error.get("code") or "CLI_FAILED"),
                    redact_text(
                        str(raw_error.get("message") or "read command failed"),
                        (self.settings.token,),
                    ),
                )
            data = response.get("data")
            if not isinstance(data, dict):
                data = {"result": data}
            return GatewayResponse(data, int((time.perf_counter() - started) * 1000))

    async def _ensure_read_process(self) -> asyncio.subprocess.Process:
        if self._read_process is not None and self._read_process.returncode is None:
            return self._read_process
        try:
            self._read_process = await asyncio.create_subprocess_exec(
                *self.command,
                "read-session",
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.DEVNULL,
                env=self._environment(),
                limit=self.settings.max_output_bytes + 1,
            )
        except FileNotFoundError as exc:
            raise GatewayError(
                "CLI_NOT_FOUND", f"apifox-cli executable was not found: {self.command[0]}"
            ) from exc
        return self._read_process

    async def _stop_read_process(self) -> None:
        process = self._read_process
        self._read_process = None
        if process is None or process.returncode is not None:
            return
        process.terminate()
        try:
            await asyncio.wait_for(process.wait(), timeout=2)
        except TimeoutError:
            process.kill()
            await process.wait()

    async def invalidate_read_cache(self) -> None:
        async with self._read_lock:
            await self._stop_read_process()

    async def close(self) -> None:
        await self.invalidate_read_cache()

    async def _invoke(
        self,
        args: tuple[str, ...],
        stdin_payload: dict[str, Any] | None,
        *,
        expect_json: bool,
    ) -> GatewayResponse:
        input_bytes = b""
        if stdin_payload is not None:
            input_bytes = json.dumps(
                stdin_payload, ensure_ascii=False, separators=(",", ":")
            ).encode("utf-8")
        if len(input_bytes) > self.settings.max_input_bytes:
            raise GatewayError(
                "PAYLOAD_TOO_LARGE",
                f"input exceeds {self.settings.max_input_bytes} bytes",
            )

        started = time.perf_counter()
        async with self._semaphore:
            try:
                process = await asyncio.create_subprocess_exec(
                    *self.command,
                    *args,
                    stdin=asyncio.subprocess.PIPE if stdin_payload is not None else None,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.PIPE,
                    env=self._environment(),
                )
            except FileNotFoundError as exc:
                raise GatewayError(
                    "CLI_NOT_FOUND",
                    f"apifox-cli executable was not found: {self.command[0]}",
                ) from exc
            except OSError as exc:
                raise GatewayError("CLI_START_FAILED", str(exc), retryable=True) from exc

            try:
                stdout, stderr = await asyncio.wait_for(
                    process.communicate(input_bytes if stdin_payload is not None else None),
                    timeout=self.settings.cli_timeout_seconds,
                )
            except TimeoutError as exc:
                process.kill()
                await process.wait()
                raise GatewayError(
                    "CLI_TIMEOUT",
                    f"apifox-cli exceeded {self.settings.cli_timeout_seconds} seconds",
                    retryable=True,
                ) from exc
            except asyncio.CancelledError:
                process.kill()
                await process.wait()
                raise

        duration_ms = int((time.perf_counter() - started) * 1000)
        if (
            len(stdout) > self.settings.max_output_bytes
            or len(stderr) > self.settings.max_output_bytes
        ):
            raise GatewayError(
                "CLI_OUTPUT_TOO_LARGE",
                f"apifox-cli output exceeds {self.settings.max_output_bytes} bytes",
            )

        stdout_text = stdout.decode("utf-8", errors="replace").strip()
        stderr_text = redact_text(
            stderr.decode("utf-8", errors="replace").strip(),
            (self.settings.token,),
        )
        if process.returncode != 0:
            error_code = "CLI_FAILED"
            message = stderr_text or redact_text(stdout_text, (self.settings.token,))
            details: dict[str, Any] | None = None
            if stdout_text:
                try:
                    failure_payload = json.loads(stdout_text)
                except json.JSONDecodeError:
                    failure_payload = None
                if isinstance(failure_payload, dict):
                    raw_error = failure_payload.get("error")
                    if isinstance(raw_error, dict):
                        raw_code = raw_error.get("code")
                        raw_message = raw_error.get("message")
                        if isinstance(raw_code, str) and _ERROR_CODE_RE.fullmatch(raw_code):
                            error_code = raw_code
                        if isinstance(raw_message, str) and raw_message.strip():
                            message = redact_text(raw_message.strip(), (self.settings.token,))
                        details = {
                            key: value
                            for key, value in failure_payload.items()
                            if key != "error"
                        }
            if error_code == "CLI_FAILED" and _VALIDATION_FAILURE_RE.search(message):
                error_code = "VALIDATION_FAILED"
            status_match = _HTTP_STATUS_RE.search(message)
            apifox_status = int(status_match.group(1)) if status_match else None
            retryable = apifox_status == 429 or bool(apifox_status and apifox_status >= 500)
            raise GatewayError(
                error_code,
                message[:2000] or f"apifox-cli exited with code {process.returncode}",
                retryable=retryable,
                exit_code=process.returncode,
                apifox_status=apifox_status,
                details=details,
            )

        if not expect_json:
            return GatewayResponse({"text": stdout_text}, duration_ms)
        if not stdout_text:
            raise GatewayError("INVALID_CLI_OUTPUT", "apifox-cli returned empty output")
        try:
            decoded = json.loads(stdout_text)
        except json.JSONDecodeError as exc:
            raise GatewayError(
                "INVALID_CLI_OUTPUT",
                "apifox-cli did not return valid JSON",
                details={"output": redact_text(stdout_text[:500], (self.settings.token,))},
            ) from exc
        if not isinstance(decoded, dict):
            decoded = {"result": decoded}
        if failure := _business_failure(decoded):
            message, apifox_status = failure
            raise GatewayError(
                "APIFOX_OPERATION_FAILED",
                redact_text(message, (self.settings.token,))[:2000],
                retryable=apifox_status == 429
                or bool(apifox_status and apifox_status >= 500),
                apifox_status=apifox_status,
                details={key: value for key, value in decoded.items() if key != "error"},
            )
        return GatewayResponse(decoded, duration_ms)
