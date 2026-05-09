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

## Tool Handler Quirk

`mcp-go` v0.52.0 stores `Arguments` as `any` (not `map[string]any`). Use `request.RequireString(key)` / `request.GetString(key, default)` instead of direct map indexing (`tools.go:63`).

## Sidecar Architecture

- `llama-server.exe` runs as a subprocess — NOT a Go binding
- llama-server only accepts images via `data:image/...;base64,...` (not file paths in API)
- Health check: polls `GET /health`, 60s timeout (`llamaserver/server.go:64`)
- Endpoint: `POST /v1/chat/completions` (OpenAI-compatible, NOT `/v1/generate`)

## Platform-Specific Code

- `installer/path_windows.go` — Windows PATH via `HKCU\Environment` registry + `WM_SETTINGCHANGE` broadcast
- `installer/path_unix.go` — appends to `~/.bashrc` or `~/.zshrc`
- Binaries need `--mmproj` flag — only certain llama.cpp builds support multimodal

## Hardware Detection

- RAM: `gopsutil/v3/mem.VirtualMemory()`
- GPU/VRAM: `nvidia-smi --query-gpu=memory.total,driver_version,name --format=csv,noheader` → output `8120 MiB, 572.83`
- Parsing: trim ` MiB` suffix, split by comma
- MiB → bytes: multiply by 1,048,576

## Config

- Primary: `~/.go-vision-mcp/config.json`
- Portable fallback: `vision-mcp.json` (same dir as binary)
- `mmproj` is model-family specific — changing `repo_id` requires matching mmproj
- Download uses temp-file + rename pattern (`.tmp` → final)

## Dependencies

| Module | Purpose |
|--------|---------|
| `mark3labs/mcp-go` | MCP protocol SDK |
| `shirou/gopsutil/v3` | Hardware detection |
| `charmbracelet/bubbletea` | TUI wizard |
| `charmbracelet/bubbles` | TUI widgets |
| `charmbracelet/lipgloss` | TUI styling |
| `golang.org/x/sys` | Windows registry API |

## Wizard (TUI)

Single Bubble Tea model (`setup/wizard.go`), 5 steps switched via `model.step` field (not separate sub-models).

## MCP Tools

| Tool | Params |
|------|--------|
| `analyze_image` | prompt (string, required), image (string, required) |
| `describe_image` | image (string, required), detail ("brief"/"detailed", optional) |

## Test Quirks

- `mcp.Tools()` is unexported — test tool registration by checking no error, not by reading back
- `llamaserver` health wait test takes 5s (actual timeout wait)
- Mock HTTP server in `mcp` tests responds to `/v1/chat/completions` with `chatResponse` JSON
- Windows: path tests use `strings.HasSuffix` instead of exact match (platform separator differences)
