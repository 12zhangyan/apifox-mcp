#!/usr/bin/env node
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawn } = require("node:child_process");

if (process.platform !== "win32" || process.arch !== "x64") {
  console.error(`@iwen-conf/apifox-mcp supports Windows x64; got ${process.platform}/${process.arch}`);
  process.exit(1);
}

const packageRoot = path.resolve(__dirname, "..");
const mcpExecutable = path.join(packageRoot, "vendor", "apifox-mcp.exe");
const cliExecutable = path.join(packageRoot, "vendor", "apifox-cli.exe");

for (const executable of [mcpExecutable, cliExecutable]) {
  if (!fs.existsSync(executable)) {
    console.error(`Incomplete @iwen-conf/apifox-mcp installation: missing ${executable}`);
    process.exit(1);
  }
}

const child = spawn(mcpExecutable, process.argv.slice(2), {
  stdio: "inherit",
  windowsHide: true,
  env: {
    ...process.env,
    APIFOX_CLI_PATH: cliExecutable,
  },
});

child.on("error", (error) => {
  console.error(`Failed to start Apifox MCP: ${error.message}`);
  process.exitCode = 1;
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exitCode = code ?? 1;
});
