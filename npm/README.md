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
