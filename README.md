# Vision MCP

A Go-based MCP (Model Context Protocol) server that enables vision analysis for LLMs without native vision capabilities using a local vision model via llama.cpp.

## Features

- **Four MCP tools** — `analyze_image`, `describe_image`, `analyze_clipboard`, `describe_clipboard` (Windows)
- **Lazy loading** — model downloads + llama-server starts only on first tool call, saves bandwidth/VRAM when idle
- **Auto-download** — downloads models from HuggingFace and llama-server binaries on demand
- **Hardware-aware** — auto-detects GPU (CUDA/Metal/Vulkan) and recommends optimal quantization
- **Automatic resume** — interrupted downloads resume from where they left off
- **Idle timeout** — automatically unloads model from GPU memory after configurable inactivity (default: 5 min), reloads on next tool call
- **Multiple clients** — concurrent MCP clients (Kilo Code, OpenCode, etc.) share the same llama-server; automatic recovery if another client stops it
- **Configurable** — supports manual model paths, LM Studio models, and custom llama-server binaries
- **TUI wizard** — Bubble Tea interactive setup wizard for guided configuration
- **Graceful shutdown** — kills llama-server on client disconnect (no orphaned processes on Windows)

## System Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| RAM | 6 GB | 16 GB |
| VRAM (GPU) | 2 GB | 8 GB (CUDA) |
| Disk | 5 GB free | 10 GB free |
| Network | Required for initial model download | |

## Quick Start

```bash
# Build from source
go build -o vision-mcp.exe .

# Run the setup wizard
vision-mcp.exe --configure

# Or use quick setup (auto-detect + download)
vision-mcp.exe
# Select option 1 from the welcome menu

# Start the MCP server
vision-mcp.exe
```

## CLI Reference

| Flag | Description |
|------|-------------|
| (none) | Start MCP server (STDIO mode) |
| `--configure` | Open interactive TUI setup wizard |
| `--install` | Quick non-interactive install with auto-detected defaults |
| `--manual` | Manual config wizard (LM Studio, custom paths) |
| `--free` | Free GPU memory — stop llama-server on port 8001 |
| `--status` | Show hardware, model, and config status |
| `--download` | Download/verify models without starting server |
| `--uninstall` | Remove installation directory |
| `--generate-agent-config` | Generate markdown with setup instructions for your agent |
| `--version` | Show version |

## Configuration

Configuration is stored at `~/.go-vision-mcp/config.json` (Windows: `%USERPROFILE%\.go-vision-mcp\config.json`).

### Fields

| Field | Default | Description |
|-------|---------|-------------|
| `repo_id` | `unsloth/Qwen3.5-4B-GGUF` | HuggingFace repo for GGUF model |
| `quantization` | `UD-IQ3_XXS` | Quantization variant to download |
| `mmproj` | `mmproj-F16.gguf` | Multimodal projector filename in the repo |
| `llama_backend` | `cuda` | Compute backend: `cuda`, `cpu`, `vulkan`, `metal` |
| `llama_bin` | `llama-server.exe` | llama-server binary name |
| `models_dir` | `~/.go-vision-mcp/models` | Directory for downloaded models |
| `port` | `8001` | Port for llama-server |
| `n_ctx` | `8192` | Context size (tokens) |
| `ngl` | `999` | GPU layers (-ngl). 0 = CPU only |
| `flash_attn` | `true` | Enable flash attention (`-fa on`) |
| `auto_download` | `true` | Download model/mmproj automatically |
| `download_mirror` | `https://github.com/ggml-org/llama.cpp/releases` | Mirror for llama-server binary |
| `custom_prompt` | `Analyze this image and respond to: %s` | Custom system prompt template |
| `kv_cache_type_k` | `q4_0` | KV cache key quantization type (`-ctk`) |
| `kv_cache_type_v` | `q4_0` | KV cache value quantization type (`-ctv`) |
| `idle_timeout` | `5` | Minutes of inactivity before unloading model (0 = disabled) |
| `model_path` | `""` | Override: exact path to model GGUF file |
| `mmproj_path` | `""` | Override: exact path to mmproj file |
| `llama_server_path` | `""` | Override: exact path to llama-server binary |
| `llama_server_mode` | `""` | Mode: `""` (PATH then download), `"auto"` (download), `"custom"` (use path) |

### Override behavior

When `model_path`, `mmproj_path`, or `llama_server_path` are set to a non-empty path, auto-download is skipped and the specified file is used directly.

`llama_server_mode` controls binary resolution:
- `""` (empty) — search PATH first, download if not found
- `"auto"` — always download regardless of PATH
- `"custom"` — use the exact path from `llama_server_path`

## Available Tools

| Tool | Parameters | Description |
|------|-----------|-------------|
| `analyze_image` | `prompt` (required), `image` (required) | Analyze an image with a custom prompt |
| `describe_image` | `image` (required), `detail` (optional) | Describe an image (brief/detailed) |
| `analyze_clipboard` | `prompt` (required) | Analyze the image in the clipboard with a custom prompt (Windows) |
| `describe_clipboard` | `detail` (optional) | Describe the image in the clipboard (Windows) |


### Image References

The `image` parameter accepts:

- **HTTP/HTTPS URLs** — `https://example.com/image.jpg`
- **Local file paths** — `C:\Users\...\image.png`
- **Data URIs** — `data:image/png;base64,...`
- **`file:///` URIs** — `file:///C:/image.png`

## MCP Client Configuration

### Kilo Code

Add to your project's `.kilocode/mcp.json`:

```json
{
  "mcpServers": {
    "vision-mcp": {
      "command": "C:\\path\\to\\vision-mcp.exe"
    }
  }
}
```

### OpenCode

Add to `~/.config/opencode/opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "vision-mcp": {
      "type": "local",
      "command": ["C:\\path\\to\\vision-mcp.exe"],
      "enabled": true
    }
  }
}
```

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "vision-mcp": {
      "command": "C:\\path\\to\\vision-mcp.exe"
    }
  }
}
```

### Cline / Roo Code / Continue

In `.vscode/mcp.json`:

```json
{
  "mcpServers": {
    "vision-mcp": {
      "command": "C:\\path\\to\\vision-mcp.exe"
    }
  }
}
```

### PI Agent

In your project's MCP configuration:

```json
{
  "mcpServers": {
    "vision-mcp": {
      "command": "C:\\path\\to\\vision-mcp.exe"
    }
  }
}
```

## Development

```bash
# Build
go build -o vision-mcp.exe .

# Run all tests
go test ./internal/...

# Lint
go vet ./...

# Format
go fmt ./...
```

## Changing Models

To use a different GGUF model (e.g., Llama, Mistral), edit `config.json`:

```json
{
  "repo_id": "other-user/other-model-GGUF",
  "quantization": "Q4_K_M",
  "mmproj": "mmproj-F16.gguf"
}
```

The server will automatically download the new model files on next start. Make sure the mmproj file is correct for your model architecture.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "llama-server not found" | Run without flags — it auto-downloads. Or place llama-server.exe in PATH or `~/.go-vision-mcp/` |
| "llama-server health check timeout" | Check that the model file exists at the path shown in `--status`. Try `--free` and restart. |
| "CUDA error" | Ensure NVIDIA drivers are installed. Run `nvidia-smi` to verify. |
| "Out of memory" | Use a lower quantization (`Q3_K_M` or `Q2_K`) in config.json |
| "Model not downloading" | Check internet connection. Files download from HuggingFace. |
| Clipboard says "no image found" | Copy an image first (screenshot with Snipping Tool, or copy image from browser) |
| Port 8001 already in use | Run `vision-mcp --free` to kill existing llama-server |

## License

MIT
