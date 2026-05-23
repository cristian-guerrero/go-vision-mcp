#!/usr/bin/env node

var fs = require('fs');
var path = require('path');

var VERSION = process.argv[2];
var BUILD_DIR = process.argv[3] || '.';

if (!VERSION) {
  console.error('Usage: node build-platform-packages.js <version> [build-dir]');
  process.exit(1);
}

var ROOT = path.join(__dirname, '..');
var TEMPLATE_DIR = path.join(ROOT, 'template-platform');

var PLATFORMS = [
  { name: 'go-vision-mcp-win32-x64',   os: 'win32',  arch: 'x64',   ext: '.exe' },
  { name: 'go-vision-mcp-win32-arm64',  os: 'win32',  arch: 'arm64', ext: '.exe' },
  { name: 'go-vision-mcp-darwin-x64',   os: 'darwin', arch: 'x64',   ext: ''     },
  { name: 'go-vision-mcp-darwin-arm64', os: 'darwin', arch: 'arm64', ext: ''     },
  { name: 'go-vision-mcp-linux-x64',    os: 'linux',  arch: 'x64',   ext: ''     },
  { name: 'go-vision-mcp-linux-arm64',  os: 'linux',  arch: 'arm64', ext: ''     },
];

var templatePkg = fs.readFileSync(path.join(TEMPLATE_DIR, 'package.json'), 'utf8');
var templateIndex = fs.readFileSync(path.join(TEMPLATE_DIR, 'index.js'), 'utf8');

PLATFORMS.forEach(function (p) {
  var dir = path.join(ROOT, p.name);
  var binDir = path.join(dir, 'bin');
  fs.mkdirSync(binDir, { recursive: true });

  var pkgJson = templatePkg
    .replace(/\{\{NAME\}\}/g, p.name)
    .replace(/\{\{VERSION\}\}/g, VERSION)
    .replace(/\{\{OS\}\}/g, p.os)
    .replace(/\{\{ARCH\}\}/g, p.arch);

  fs.writeFileSync(path.join(dir, 'package.json'), pkgJson);
  fs.writeFileSync(path.join(dir, 'index.js'), templateIndex);

  var srcBin = path.join(BUILD_DIR, p.name + p.ext);
  var dstBin = path.join(binDir, 'vision-mcp' + p.ext);
  if (!fs.existsSync(srcBin)) {
    console.error('Warning: binary not found: ' + srcBin);
    return;
  }
  fs.copyFileSync(srcBin, dstBin);
  fs.chmodSync(dstBin, 0o755);

  console.log('Created: ' + dir);
});
