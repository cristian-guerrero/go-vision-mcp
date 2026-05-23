var path = require('path');

var PLATFORM_PACKAGES = {
  'win32-x64': 'go-vision-mcp-win32-x64',
  'win32-arm64': 'go-vision-mcp-win32-arm64',
  'darwin-x64': 'go-vision-mcp-darwin-x64',
  'darwin-arm64': 'go-vision-mcp-darwin-arm64',
  'linux-x64': 'go-vision-mcp-linux-x64',
  'linux-arm64': 'go-vision-mcp-linux-arm64',
};

var key = process.platform + '-' + process.arch;
var pkg = PLATFORM_PACKAGES[key];
if (!pkg) {
  throw new Error(
    'Unsupported platform: ' + process.platform + ' ' + process.arch +
    '. Supported platforms: win32 (x64, arm64), darwin (x64, arm64), linux (x64, arm64).'
  );
}

var pkgDir;
try {
  pkgDir = path.dirname(require.resolve(pkg + '/package.json'));
} catch (e) {
  throw new Error(
    'Missing platform dependency: ' + pkg + '. ' +
    'Run npm install --include=optional to install it, or check your platform support.'
  );
}

var ext = process.platform === 'win32' ? '.exe' : '';
var binaryPath = path.join(pkgDir, 'bin', 'vision-mcp' + ext);

exports.binaryPath = binaryPath;
