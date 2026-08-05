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

On Windows, prefer the installed `apifox-mcp.cmd` / `apifox-cli.cmd` shims or explicit executable paths when another shell cannot resolve `npx`. Stdout is reserved for MCP/JSON output; diagnostics are written to stderr.
