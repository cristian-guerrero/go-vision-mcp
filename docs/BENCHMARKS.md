# Benchmarks

Performance comparison of supported vision models on a **Windows** system with **NVIDIA RTX 3080 16GB VRAM (laptop)**.

## Test Setup

| Parameter | Value |
|-----------|-------|
| **Hardware** | NVIDIA RTX 3080 16GB VRAM (laptop) |
| **OS** | Windows |
| **llama-server** | Latest CUDA build (auto-downloaded) |
| **Backend** | CUDA |
| **Flash Attention** | Enabled |
| **Context Size** | 8192 |
| **KV Cache** | q4_0 (K and V) |
| **Image** | Windows screenshot, 1088×552 px |
| **Image complexity** | High — screenshot of OpenCode IDE showing multiple panels: file explorer tree with Go source files, code editor with `main.go` open (imports, function signatures, `runWizardCmd`), a terminal panel with build output, and status bar elements. Dense text content including file paths, code syntax, log messages, and UI chrome. |
| **Prompt** | "Analyze this image" (system prompt) |
| **Quantization** | IQ4_XS for 4B models, IQ4_XS for 2B |

> **Note**: All models use the same quantization family. The 2B model at IQ4_XS uses fewer bits overall since it has fewer parameters.

## Results

### Model: `unsloth/Qwen3-VL-4B-Instruct-GGUF`

| Metric | Value |
|--------|-------|
| Image processing | 1051 ms |
| Prompt eval | 1111.45 ms / 1616 tokens (0.69 ms/token, 1453.95 t/s) |
| Generation | 15116.28 ms / 886 tokens (17.06 ms/token, 58.61 t/s) |
| **Total time** | **16.23 s** / 2502 tokens |

### Model: `unsloth/Qwen3.5-4B-GGUF`

| Metric | Value |
|--------|-------|
| Image processing | 1224 ms |
| Prompt eval | 1322.41 ms / 1619 tokens (0.82 ms/token, 1224.28 t/s) |
| Generation | 16421.67 ms / 889 tokens (18.47 ms/token, 54.14 t/s) |
| **Total time** | **17.74 s** / 2508 tokens |

### Model: `unsloth/Qwen3-VL-2B-Instruct-1M-GGUF`

| Metric | Value |
|--------|-------|
| Image processing | 286 ms |
| Prompt eval | 321.89 ms / 596 tokens (0.54 ms/token, 1851.56 t/s) |
| Generation | 2934.77 ms / 439 tokens (6.69 ms/token, 149.59 t/s) |
| **Total time** | **3.26 s** / 1035 tokens |

## Comparison

| Metric | VL 4B | Qwen3.5 4B | VL 2B 1M |
|--------|-------|------------|----------|
| Image processing | 1051 ms | 1224 ms | **286 ms** |
| Prompt eval | 0.69 ms/tok | 0.82 ms/tok | **0.54 ms/tok** |
| Generation speed | 58.61 t/s | 54.14 t/s | **149.59 t/s** |
| **Total time** | **16.23 s** | **17.74 s** | **3.26 s** |
| Tokens generated | 886 | 889 | 439 |

## Observations

- **VL 4B** is slightly faster than Qwen3.5 4B across all metrics (~9% total time improvement) while maintaining the same output quality.
- **VL 2B 1M** is ~5× faster than both 4B models, making it ideal for quick analyses where latency matters. Responses are shorter (~half the tokens) but consistently accurate.
- Image processing benefits significantly from the Qwen3-VL architecture: VL 4B is ~14% faster than Qwen3.5 4B, and VL 2B is ~4× faster than both.
- For interactive MCP tool calls where the user waits for a response, the 2B model provides near-instant results while the 4B models take ~16-18 seconds.
- Response quality was consistent across all three models for the tested prompt — all correctly identified the code analysis context.

## Recommendation

| Use case | Model |
|----------|-------|
| Maximum quality, willing to wait | `Qwen3-VL-4B-Instruct-GGUF` |
| Balance of speed and quality | `Qwen3.5-4B-GGUF` |
| Fast responses, low latency | `Qwen3-VL-2B-Instruct-1M-GGUF` |
