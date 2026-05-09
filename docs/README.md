# Vision MCP Server

A Go-based MCP (Model Context Protocol) server that enables vision analysis for LLMs without native vision capabilities (DeepSeek, etc.) using a local Qwen3.5-4B vision model.

## System Requirements

| Requirement | Minimum | Recommended |
|------------|---------|-------------|
| RAM | 6 GB | 16 GB |
| VRAM (GPU) | 2 GB | 8 GB (CUDA) |
| Disk | 5 GB free | 10 GB free |
| Network | Required for initial model download | |

## Quick Start

```bash
go install github.com/vision-mcp/cmd/vision-mcp@latest
vision-mcp --configure   # Interactive setup wizard
vision-mcp               # Start the server
```

## CLI Reference

| Flag | Description |
|------|-------------|
| (none) | Start MCP server in STDIO mode |
| `--configure` | Open interactive TUI setup wizard |
| `--install` | Quick non-interactive install with auto-detected defaults |
| `--uninstall` | Remove installation directory |
| `--status` | Show hardware, model, and config status |
| `--download` | Download/verify models without starting server |
| `--generate-agent-config` | Generate markdown file with setup instructions for your agent |
| `--version` | Show version |

## Configuration

Configuration is stored at `~/.go-vision-mcp/config.json` (Windows: `%USERPROFILE%\.go-vision-mcp\config.json`).

```json
{
  "repo_id": "unsloth/Qwen3.5-4B-GGUF",
  "quantization": "Q4_K_M",
  "mmproj": "mmproj-F16.gguf",
  "llama_backend": "cuda",
  "llama_bin": "llama-server.exe",
  "models_dir": "~/.go-vision-mcp/models",
  "port": 8001,
  "n_ctx": 8192,
  "ngl": 99,
  "flash_attn": true,
  "auto_download": true,
  "custom_prompt": "Analyze this image and respond to: %s"
}
```

## Available Tools

### `analyze_image`

Analyze an image with a custom prompt using the local vision model.

**Parameters:**
- `prompt` (string, required): Question or instruction about the image
- `image` (string, required): URL (http/https), local file path, or base64 data URI

### `describe_image`

Get a visual description of an image.

**Parameters:**
- `image` (string, required): URL, file path, or data URI
- `detail` (string, optional): "brief" or "detailed" (default: "detailed")

## MCP Client Configuration

### Claude Desktop

```json
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
```

### Kilo Code / OpenCode / PI Agent

```json
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
```

### Cline / Roo Code / Continue

In `.vscode/mcp.json`:

```json
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
```

## Changing Models

To use a different GGUF model from HuggingFace, edit `config.json`:

```json
{
  "repo_id": "other-user/other-model-GGUF",
  "quantization": "Q4_K_M",
  "mmproj": "mmproj-F16.gguf"
}
```

The server will automatically download the new model files on next start.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "llama-server not found" | Download from https://github.com/ggml-org/llama.cpp/releases and place in install directory or PATH |
| "CUDA error" | Ensure NVIDIA drivers are installed. Run `nvidia-smi` to verify. |
| "Out of memory" | Use a lower quantization (e.g., `Q3_K_M`) in config.json |
| "Model not downloading" | Check internet connection. Files download from HuggingFace. |
| "Image processing requires a vision model" | Ensure you're using a valid mmproj for your model family |

## Build from Source

```bash
git clone https://github.com/vision-mcp/vision-mcp.git
cd vision-mcp
make build
```

## License

MIT
