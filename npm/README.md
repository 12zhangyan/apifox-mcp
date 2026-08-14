# @yanzhang123/apifox-mcp

Windows x64 npm distribution of the enterprise Apifox MCP server. The package bundles both the
MCP runtime and `apifox-cli`; Go, Python, and uv are not required at runtime.

```powershell
npm install -g @yanzhang123/apifox-mcp

$env:APIFOX_TOKEN = "your-token"
$env:APIFOX_PROJECT_ID = "your-project-id"
apifox-mcp
```

Writes default to plan-only. Set `APIFOX_MCP_WRITE_MODE=apply` only in an approved environment.
The CLI timeout defaults to 120 seconds and can be changed with `APIFOX_MCP_CLI_TIMEOUT`.
Read tools reuse endpoint/OpenAPI caches for 300 seconds by default; configure
`APIFOX_MCP_READ_CACHE_TTL` to change the TTL. Successful applies invalidate the cache.

On Windows, prefer the installed `apifox-mcp.cmd` / `apifox-cli.cmd` shims or explicit executable paths when another shell cannot resolve `npx`. Stdout is reserved for MCP/JSON output; diagnostics are written to stderr.

Do not configure Cursor with a path inside `node_modules/@yanzhang123/apifox-mcp`; that path
moves when NVM switches Node versions. Run `where.exe apifox-mcp.cmd` after switching versions
and configure the returned shim path (or the shim name when Cursor inherits the same `PATH`).
