// Package installer — README generation for installed directory.
package installer

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateReadme writes a README.md into the install directory with
// instructions for configuring various MCP clients.
func GenerateReadme(installDir string) error {
	content := fmt.Sprintf(`# Vision MCP - Installed in this directory

## How to use with your favorite MCP client

### OpenCode
Create or edit ~/.opencode/mcp.json:
` + "```json" + `
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
` + "```" + `

### PI Agent
In your project or global config, add:
` + "```json" + `
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
` + "```" + `

### Kilo Code
Add to your MCP configuration:
` + "```json" + `
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
` + "```" + `

### Claude Desktop
Edit claude_desktop_config.json:
` + "```json" + `
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
` + "```" + `

### Cline / Roo Code / Continue
In .vscode/mcp.json or settings:
` + "```json" + `
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
` + "```" + `

### Manual (terminal)
` + "```bash" + `
vision-mcp
` + "```" + `
The server starts in STDIO mode, waiting for client connection.

---

## Configuration

Edit ` + "`config.json`" + ` in this same directory to change model, quantization, port, etc.

### Change model
` + "```json" + `
{
  "repo_id": "another-user/another-model-GGUF",
  "quantization": "Q4_K_M",
  "mmproj": "mmproj-F16.gguf"
}
` + "```" + `

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "llama-server not found" | Download llama-server from https://github.com/ggml-org/llama.cpp/releases and place it in this directory or in your PATH |
| "CUDA error" | Ensure NVIDIA drivers are installed. Run nvidia-smi to verify. |
| "Out of memory" | Use a lower quantization (Q3_K_M) in config.json |
| "Model not downloading" | Check internet connection. Files are downloaded from HuggingFace. |
`)

	path := filepath.Join(installDir, "README.md")
	return os.WriteFile(path, []byte(content), 0644)
}
