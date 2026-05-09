# Vision MCP - Architecture

## Overview

Vision MCP is a Go-based MCP server that provides vision capabilities to LLMs without native vision support. It achieves this by running a local `llama-server` instance with a multimodal GGUF model (Qwen3.5-4B).

## Architecture Decision: Sidecar vs Direct Binding

### Why sidecar?

- Neither `node-llama-cpp` nor Go bindings for llama.cpp support `--mmproj` (multimodal projection)
- `llama-server` provides an OpenAI-compatible HTTP API
- The sidecar pattern isolates the inference engine from the MCP server
- Graceful shutdown and process lifecycle management are straightforward

## Component Diagram

```
┌────────────────────┐
│  MCP Client        │
│  (DeepSeek/Claude) │
└────────┬───────────┘
         │ STDIO (MCP Protocol)
         ▼
┌────────────────────┐
│  MCP Server (Go)   │
│                    │
│  ├─ Config loader  │
│  ├─ Tool handlers  │
│  ├─ Image resolver │
│  └─ HTTP client    │
└────────┬───────────┘
         │ HTTP POST /v1/chat/completions
         ▼
┌─────────────────────┐
│  llama-server       │
│  (subprocess)       │
│                     │
│  • Localhost:8001   │
│  • Qwen3.5-4B-GGUF  │
│  • + mmproj         │
│  • KV cache         │
│  • CUDA/Cpu/Metal   │
└─────────────────────┘
```

## Data Flow

### 1. Startup Sequence

```
1. Load config.json (or create defaults)
2. Detect hardware (RAM, VRAM, disk)
3. Recommend quantization based on hardware
4. Save updated config
5. Download model.gguf + mmproj from HuggingFace (if missing)
6. Spawn llama-server as subprocess
7. Health check loop (polling /health)
8. Register MCP tools
9. Begin STDIO MCP loop
```

### 2. Tool Call: analyze_image

```
1. Client sends: { "name": "analyze_image", "arguments": { "prompt": "...", "image": "..." } }
2. Server resolves image:
   - URL → HTTP GET → base64 encode
   - File path → read file → base64 encode
   - data: URI → pass through
3. Construct OpenAI-compatible chat completion request:
   {
     "messages": [{
       "role": "user",
       "content": [
         { "type": "image_url", "image_url": { "url": "data:image/png;base64,..." } },
         { "type": "text", "text": "prompt text" }
       ]
     }]
   }
4. POST to llama-server /v1/chat/completions
5. Parse JSON response, extract content
6. Return to client as MCP ToolResult
```

### 3. Shutdown Sequence

```
1. SIGINT/SIGTERM received
2. Cancel context (propagates to llama-server subprocess)
3. Kill llama-server process
4. Exit with code 0
```

## Package Structure

| Package | Responsibility |
|---------|---------------|
| `config` | JSON config load/save, defaults, model paths |
| `hardware` | Hardware detection (RAM, VRAM, disk), quantization recommendation |
| `download` | HTTP file download with progress, HuggingFace URL builder |
| `llamaserver` | Spawn/manage llama-server subprocess, health check |
| `image` | Image resolution: URL/path → base64 data URI |
| `mcp` | MCP tool definitions and handlers |
| `installer` | Binary installation, PATH management, README generation |
| `setup` | Bubble Tea TUI wizard for interactive configuration |
| `agentconfig` | Generate markdown file with setup instructions for agents |

## Error Handling Strategy

- MCP tool errors are returned as `ToolResult` with `isError: true` (not as protocol-level errors)
- llama-server connection errors bubble up to the MCP tool handler
- Missing model files trigger auto-download (if `auto_download` is enabled)
- Health check timeout is configurable (default: 60s)
- Network errors during download result in partial file cleanup (temp file → rename pattern)

## Edge Cases

| Scenario | Handling |
|----------|----------|
| Download interrupted | Temp file pattern: download to `.tmp`, rename on completion. Verify Content-Length. |
| llama-server crash | Context propagation from parent. Re-spawn on next tool call |
| Invalid image input | Return tool error: "Failed to resolve image: ..." |
| Out of memory (OOM) | Recommend lower quantization. User must restart with new config. |
| PATH corruption (Windows) | Read existing PATH from registry, append, validate. No destructive operations. |
| Multiple instances | Each binds to its configured port. Detect port conflict. |
