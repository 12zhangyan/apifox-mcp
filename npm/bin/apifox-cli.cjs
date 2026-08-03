#!/usr/bin/env node
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawn } = require("node:child_process");

if (process.platform !== "win32" || process.arch !== "x64") {
  console.error(`@yanzhang123/apifox-mcp supports Windows x64; got ${process.platform}/${process.arch}`);
  process.exit(1);
}

const executable = path.resolve(__dirname, "..", "vendor", "apifox-cli.exe");
if (!fs.existsSync(executable)) {
  console.error(`Incomplete @yanzhang123/apifox-mcp installation: missing ${executable}`);
  process.exit(1);
}

const child = spawn(executable, process.argv.slice(2), {
  stdio: "inherit",
  windowsHide: true,
});

child.on("error", (error) => {
  console.error(`Failed to start apifox-cli: ${error.message}`);
  process.exitCode = 1;
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exitCode = code ?? 1;
});
