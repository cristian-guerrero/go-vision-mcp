# Vision MCP Agent Guide

## Entrypoint

```go
package main  // main.go at root (not cmd/)
```

Build: `go build -o vision-mcp.exe .`

## Commands

```bash
make build    # go build -o vision-mcp.exe .
make test     # go test ./internal/...
make lint     # go vet ./...
make fmt      # go fmt ./...
make test-all # go test ./... (all tests, no tag filtering)
make tidy     # go mod tidy
```

Integration tests: `go test -tags=integration ./internal/...`

No test runner config needed — pure `testing` stdlib.

## Application Flow

```
main()
├── flag dispatch (--configure, --install, --status, --free, etc.)
├── MCP STDIO mode (no flags):
│   ├── if not interactive → runServer() directly (MCP client)
│   ├── if interactive + no config → showWelcomeMenu() (Bubble Tea TUI)
│   └── if interactive + config exists → runServer()
```

### runServer() — Async Init

MCP server starts immediately on STDIO to respond to handshake. Model download and llama-server startup run in a background goroutine:

1. Load config
2. Detect hardware, apply defaults
3. Download model + mmproj (if missing)
4. Find llama-server (PATH → download → install dir)
5. Check if port 8001 already has llama-server → reuse if yes
6. Start llama-server, wait for health
7. Signal tools as ready

Tools block on `waitReady()` until llama-server responds. Multiple MCP clients share the same llama-server instance.

## Tool Handler Quirk

`mcp-go` v0.52.0 stores `Arguments` as `any` (not `map[string]any`). Use `request.RequireString(key)` / `request.GetString(key, default)` instead of direct map indexing (`tools.go:63`).

## MCP Tools

| Tool | Params | Description |
|------|--------|-------------|
| `analyze_image` | prompt (required), image (required) | Analyze an image with custom prompt |
| `describe_image` | image (required), detail (optional) | Describe an image |
| `get_clipboard_image` | - | Get image from Windows clipboard as data URI |
| `describe_clipboard` | detail (optional) | Describe the image in the clipboard |

All tools wait for llama-server to be ready before processing. Clipboard tools use PowerShell `System.Windows.Forms.Clipboard` on Windows, trying GetImage → GetFileDropList → GetData("Bitmap").

## Sidecar Architecture

- `llama-server.exe` runs as a subprocess — NOT a Go binding
- llama-server only accepts images via `data:image/...;base64,...` (not file paths in API)
- Health check: polls `GET /health`, 60s timeout (`llamaserver/server.go:64`)
- Endpoint: `POST /v1/chat/completions` (OpenAI-compatible, NOT `/v1/generate`)
- If port 8001 already responds to health check, reuses existing instance

## CLI Flags

| Flag | Description |
|------|-------------|
| (none) | Start MCP server (STDIO mode) |
| `--configure` | TUI setup wizard |
| `--install` | Copy binary + create launcher + detect hardware |
| `--manual` | Manual config wizard (LM Studio, custom paths) |
| `--free` | Free GPU memory — kill llama-server on port |
| `--status` | Show config, hardware, paths |
| `--download` | Download/verify models only |
| `--uninstall` | Remove install directory |
| `--generate-agent-config` | Generate markdown with MCP JSON config |
| `--version` | Show version |

## llama-server Optimizations

Args passed to llama-server:

- `-ctk q4_0 -ctv q4_0` — KV cache quantization to 4-bit
- `-fa on` — Flash attention (ignored if unsupported)
- `--chat-template-kwargs {"enable_thinking": false}` — disable thinking mode

## Config Fields

All fields are emitted in config.json even when empty, so users can manually edit:

```json
{
  "repo_id": "unsloth/Qwen3.5-4B-GGUF",
  "quantization": "Q4_K_M",
  "model_path": "/custom/path/model.gguf",
  "mmproj_path": "/custom/path/mmproj.gguf",
  "llama_server_path": "/custom/path/llama-server.exe"
}
```

`model_path`/`mmproj_path`/`llama_server_path` override auto-download when set.

## Graceful Shutdown

`llamaserver.Stop()` sends SIGTERM → waits 3 seconds → SIGKILL. On process signal, same flow runs in the signal handler.

## Download Resume

`download.DownloadFile()` checks for existing `.tmp` files and sends `Range: bytes=N-` headers. The server must support range requests (HuggingFace does). On `416 Range Not Satisfiable`, the `.tmp` is renamed to final.

## Platform-Specific

- `installer/path_windows.go` — PATH via `HKCU\Environment` registry + `WM_SETTINGCHANGE`
- `installer/path_unix.go` — appends to `~/.bashrc` or `~/.zshrc`
- `mcp/tools.go:clipboardImageDataURI()` — uses PowerShell `System.Windows.Forms.Clipboard` (Windows only)

## Hardware Detection

- RAM: `gopsutil/v3/mem.VirtualMemory()`
- GPU/VRAM: `nvidia-smi --query-gpu=memory.total,driver_version,name --format=csv,noheader`
- Parsing: trim ` MiB` suffix, split by comma
- MiB → bytes: multiply by 1,048,576

## Dependencies

| Module | Purpose |
|--------|---------|
| `mark3labs/mcp-go` | MCP protocol SDK |
| `shirou/gopsutil/v3` | Hardware detection |
| `charmbracelet/bubbletea` | TUI wizard |
| `charmbracelet/bubbles` | TUI widgets |
| `charmbracelet/lipgloss` | TUI styling |
| `golang.org/x/sys` | Windows registry API |

## Test Quirks

- `mcp.Tools()` is unexported — test tool registration by checking no error
- `llamaserver` health wait test takes 5s (actual timeout wait)
- Mock HTTP server in `mcp` tests responds to `/v1/chat/completions`
- Windows: path tests use `strings.HasSuffix` instead of exact match
