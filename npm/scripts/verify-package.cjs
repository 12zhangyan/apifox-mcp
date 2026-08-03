"use strict";

const fs = require("node:fs");
const path = require("node:path");

const packageRoot = path.resolve(__dirname, "..");
const requiredFiles = [
  "bin/apifox-mcp.cjs",
  "bin/apifox-cli.cjs",
  "vendor/apifox-mcp.exe",
  "vendor/apifox-cli.exe",
];

for (const relativePath of requiredFiles) {
  const absolutePath = path.join(packageRoot, relativePath);
  const stat = fs.statSync(absolutePath, { throwIfNoEntry: false });
  if (!stat || !stat.isFile() || stat.size === 0) {
    throw new Error(`npm package is missing non-empty ${relativePath}`);
  }
}

console.error("verified Windows npm package payload");
