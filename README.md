# Vision MCP

A Go-based MCP (Model Context Protocol) server that enables vision analysis for LLMs without native vision capabilities using a local vision model via llama.cpp.

## Features

- **Nine MCP tools** — image analysis from URLs, local files, clipboard, and clipboard history (Windows / Linux with X11 or Wayland)
- **Clipboard history monitor** — optionally polls clipboard for images, keeps a configurable history for multi-image comparison (e.g. "the first image is the before, the third is the after")
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

Requirements depend on the selected model and quantization. The default model is **Qwen3.5-4B** (~4B parameters). The tool **automatically detects your hardware** and selects the optimal quantization — no manual tuning needed.

### Guidance by VRAM (default model: Qwen3.5-4B)

| VRAM | Recommended Quantization | Approx. Size | Quality |
|------|------------------------|-------------|---------|
| ≥ 12 GB | Q8_0 / Q6_K | 3.5 – 4.5 GB | Maximum |
| ≥ 8 GB | Q5_K_M | ~3.1 GB | High |
| ≥ 6 GB | Q4_K_M | ~2.7 GB | Balanced |
| ≥ 4 GB | Q3_K_M / IQ4_XS | 2.1 – 2.5 GB | Economy |
| ≥ 2 GB | UD-IQ3_XXS | ~1.8 GB | Ultra low RAM |

- **RAM**: 8 GB+ recommended (4 GB minimum)
- **Disk**: 5 GB free for model storage
- **Network**: Required for initial model download
- **GPU**: NVIDIA (CUDA), AMD/Intel (Vulkan), or Apple Silicon (Metal)

## Quick Start

The tool automatically detects your GPU, RAM, and VRAM, then selects the best model and quantization for your system.

```bash
# Build from source
go build -o vision-mcp.exe .

# Quick setup — auto-detects hardware, downloads model, starts server
vision-mcp.exe

# Or use the interactive setup wizard
vision-mcp.exe --configure
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

Configuration is stored at `~/.go-mcp/vision/config.json` (Windows: `%USERPROFILE%\.go-mcp\vision\config.json`).

### Fields

| Field | Default | Description |
|-------|---------|-------------|
| `repo_id` | `unsloth/Qwen3.5-4B-GGUF` | HuggingFace repo for GGUF model |
| `quantization` | `UD-IQ3_XXS` | Quantization variant to download |
| `mmproj` | `mmproj-F16.gguf` | Multimodal projector filename in the repo |
| `llama_backend` | `cuda` | Compute backend: `cuda`, `cpu`, `vulkan`, `metal` |
| `llama_bin` | `llama-server.exe` | llama-server binary name |
| `models_dir` | `~/.go-mcp/vision/models` | Directory for downloaded models |
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
| `clipboard_monitor_enabled` | `false` | Enable clipboard history monitor for multi-image analysis |
| `clipboard_history_limit` | `5` | Max cached images in clipboard history (1-20) |
| `clipboard_cache_dir` | `""` | Custom cache directory for clipboard history (default: `~/.go-mcp/vision/clipboard-cache`) |
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

### Image Analysis (files, URLs, data URIs)

| Tool | Parameters | Description |
|------|-----------|-------------|
| `analyze_image` | `prompt` (required), `image` (required) | Analyze an image with a custom prompt |

### Current Clipboard

| Tool | Parameters | Description |
|------|-----------|-------------|
| `analyze_clipboard` | `prompt` (required) | Analyze the image currently in the clipboard |

### Clipboard History (requires `clipboard_monitor_enabled: true`)

The clipboard monitor runs in the background, polling the clipboard every 500ms. Each new image is detected, deduplicated by hash, and saved to a circular buffer. When the image comes from a file (copied via File Explorer), the original file path is stored — no copy is made, preserving full quality. When the image comes from a raw clipboard source (PrintScreen, browser "Copy Image"), a cached PNG is saved.

History is cleared when the server stops. Cached files are deleted; original files are never touched.

| Tool | Parameters | Description |
|------|-----------|-------------|
| `list_clipboard_history` | — | List all cached clipboard images with index, timestamp, and source path |
| `analyze_clipboard_image` | `index` (required), `prompt` (required) | Ask a custom question about a specific cached image by index |
| `analyze_clipboard_images` | `indices` (required), `prompt` (required) | Analyze multiple cached images at once (comma-separated indices, e.g. `"1,2,3"`) |

#### Multi-image example workflow

```
User copies 3 images (Ctrl+C three times)
User: "compare the first and third images"

LLM → list_clipboard_history()
LLM  ← #1 — 14:03:15 — C:\Users\...\before.png
        #2 — 14:04:22 — C:\Users\...\middle.png  
        #3 — 14:05:10 — C:\Users\...\after.png

LLM → analyze_clipboard_images(indices="1,3",
         prompt="Image #1 is the BEFORE, image #2 is the AFTER. What changed?")
LLM  ← "The before image shows an empty form. The after image shows..."
```

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
| "llama-server not found" | Run without flags — it auto-downloads. Or place llama-server.exe in PATH or `~/.go-mcp/vision/` |
| "llama-server health check timeout" | Check that the model file exists at the path shown in `--status`. Try `--free` and restart. |
| "CUDA error" | Ensure NVIDIA drivers are installed. Run `nvidia-smi` to verify. |
| "Out of memory" | Use a lower quantization (`Q3_K_M` or `Q2_K`) in config.json |
| "Model not downloading" | Check internet connection. Files download from HuggingFace. |
| Clipboard says "no image found" | Copy an image first (screenshot with Snipping Tool, or copy image from browser) |
| Clipboard history shows "Clipboard monitor not enabled" | Set `clipboard_monitor_enabled: true` in config.json, or enable it during `--configure` or `--manual` setup |
| Clipboard history empty after restart | Expected — history is cleared on server shutdown for privacy. Enable monitor and copy images again |
| Copied file image looks lower quality | When copying from a file, the monitor prefers the original file path (no quality loss). If the image was copied from a browser or screenshot (raw clipboard), a PNG conversion is used — this may alter colors or dimensions slightly |
| Port 8001 already in use | Run `vision-mcp --free` to kill existing llama-server |

## License

MIT
