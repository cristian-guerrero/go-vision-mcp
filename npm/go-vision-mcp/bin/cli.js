#!/usr/bin/env node
var cp = require('child_process');
var path = require('path');

try {
  var binaryPath = require(path.join(__dirname, '..')).binaryPath;
} catch (e) {
  console.error('Failed to resolve go-vision-mcp binary:', e.message);
  process.exit(1);
}

var args = process.argv.slice(2);
var proc = cp.spawn(binaryPath, args, { stdio: 'inherit' });
proc.on('exit', function (code) {
  process.exit(code === null ? 1 : code);
});
