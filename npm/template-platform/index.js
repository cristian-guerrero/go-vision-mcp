var path = require('path');
var binaryName = 'vision-mcp' + (process.platform === 'win32' ? '.exe' : '');
exports.binaryPath = path.join(__dirname, 'bin', binaryName);
