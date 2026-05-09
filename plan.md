# Plan: Vision MCP Server (Go)

## Objetivo

MCP server en Go que permite a modelos sin visión (DeepSeek, etc.) analizar imágenes usando Qwen3.5-4B local.

---

## Stack Tecnológico

| Componente   | Tecnología                                    |
|------------- |-----------------------------------------------|
| Lenguaje     | Go 1.23+                                      |
| MCP SDK      | `github.com/mark3labs/mcp-go`                 |
| Inference    | `llama-server.exe` como subproceso (sidecar)  |
| HTTP client  | `net/http` (stdlib)                           |
| JSON         | `encoding/json` (stdlib)                      |
| Hardware     | `gopsutil/v3` + `os/exec` (`nvidia-smi`)      |
| TUI wizard  | `github.com/charmbracelet/bubbletea`          |
| Tests        | `testing` stdlib + `testcontainers-go` (opc)  |

**Binding directo no es viable**: ni `node-llama-cpp` ni bindings Go de llama.cpp soportan `--mmproj` (multimodal). La arquitectura sidecar con `llama-server` es la solución.

---

## Arquitectura

```
Cliente MCP (DeepSeek/Claude)
    │
    │  tools/call("analyze_image", { prompt, image })
    ▼
MCP Server (Go)
    │
    │  ├─ Detecta hardware → elige quantization
    │  ├─ Descarga model.gguf + mmproj desde HF (si no existen)
    │  ├─ Spawnea llama-server.exe como subproceso
    │  └─ Escucha tool calls
    │
    ├── POST /v1/chat/completions ──→ llama-server.exe (localhost:8001)
    │     { messages: [{ content: [
    │       { type: "image_url", image_url: { url: "data:..." } },
    │       { type: "text", text: prompt }
    │     ]}]}
    │
    └── respuesta ────────────────→ Cliente MCP
```

### Flujo completo:

1. **Inicio**: detectar hardware RAM/VRAM/disco, elegir quantization, crear config si no existe
2. **Descarga**: si faltan model.gguf o mmproj, descargar desde HuggingFace con barra de progreso
3. **Spawn**: ejecutar `llama-server.exe` con `--mmproj`, esperar health check
4. **Tool call**: recibir `analyze_image(prompt, image)` del cliente
5. **Resolver imagen**: URL → descargar, path local → leer archivo, base64 → decodificar
6. **Inferir**: POST a `llama-server` con imagen como data URI base64
7. **Responder**: devolver texto generado al cliente MCP

---

## Hardware detection y quantization selector

### Detección de hardware

| Componente | Método | Código |
|-----------|--------|--------|
| RAM total/disponible | `gopsutil/v3/mem` | `mem.VirtualMemory()` |
| Disco disponible | `gopsutil/v3/disk` | `disk.Usage(modelsDir)` |
| VRAM (NVIDIA) | `os/exec: nvidia-smi` | `nvidia-smi --query-gpu=memory.total,driver_version --format=csv,noheader` |
| GPU tipo | `os/exec: nvidia-smi` | Detecta CUDA vs no GPU |
| Backend CPU | fallback | Sin GPU presente |
| Backend Vulkan | `os/exec: vulkaninfo` | Detección opcional |

### Quantization recomendada por hardware (auto-select)

| Condición | Quantización | Tamaño model | + mmproj |
|-----------|-------------|-------------|----------|
| VRAM >= 6GB o RAM >= 16GB | `Q5_K_M` | ~3.14 GB | ~3.8 GB |
| VRAM >= 4GB o RAM >= 8GB | `Q4_K_M` | ~2.74 GB | ~3.4 GB |
| VRAM >= 2GB o RAM >= 6GB | `Q3_K_M` | ~2.29 GB | ~2.9 GB |
| RAM < 6GB | `IQ4_XS` | ~2.48 GB | ~3.1 GB |
| Solo CPU, RAM >= 12GB | `Q4_K_M` (default) | ~2.74 GB | ~3.4 GB |

### Durante la instalación (wizard), el usuario puede elegir:

```
[1/4] Detectando hardware...
  • RAM: 16.0 GB
  • VRAM: 8.0 GB (NVIDIA RTX 3070, driver 572.83)
  • Backend: CUDA 12.8
  • Disco: 120 GB libres

  → Backend recomendado: CUDA
  ¿Usar CUDA? [Y/n] Y

  → Quantization recomendada: Q5_K_M (~3.14 GB + 676 MB mmproj)
  ¿Qué quantization deseas usar?
    1) Q5_K_M  (3.14 GB) ← Alta calidad [recomendado]
    2) Q4_K_M  (2.74 GB) ← Balanceado
    3) Q3_K_M  (2.29 GB) ← Económico
    4) Q8_0    (4.48 GB) ← Calidad máxima (requiere >6GB VRAM)
  Selecciona [1-4] o presiona Enter para usar el recomendado: 1
```

### Backend llama-server: detección y selección

El sistema detecta qué variante de `llama-server` se necesita y guía al usuario:

| Estado | Acción |
|--------|--------|
| NVIDIA GPU detectada | Recomendar `llama-server.exe` con CUDA. Preguntar al usuario. |
| Sin GPU NVIDIA | Recomendar `llama-server.exe` CPU. Ofrecer Vulkan como alternativa. |
| GPU AMD/Intel | Ofrecer Vulkan o CPU. |
| macOS Apple Silicon | Ofrecer Metal backend. |
| Linux | Detectar CUDA, ROCm, Vulkan según librerías instaladas. |

Si el binario no existe en el sistema, el wizard ofrece descargarlo:

```
  ¿Deseas descargar llama-server con soporte CUDA ahora?
  [Y/n] Y

  Detectando release más reciente de llama.cpp...
  ✓ Descargando llama-bXXXX-bin-win-cuda-x64.zip
  ✓ Extrayendo llama-server.exe a C:\Users\tu\.go-vision-mcp\llama-server.exe
```

Las URLs de descarga se obtienen escaneando [llama.cpp releases](https://github.com/ggml-org/llama.cpp/releases) o usando una URL hardcodeada (configurable).

### Implementación:

```go
type GPUInfo struct {
    Present     bool
    Vendor      string // "nvidia", "amd", "intel", "apple"
    DriverVer   string
    VRAM        uint64
    BackendType string // "cuda", "vulkan", "metal", "cpu"
}

type HardwareProfile struct {
    TotalRAM     uint64
    AvailableRAM uint64
    GPU          GPUInfo
    FreeDisk     uint64
}

type InstallSelection struct {
    Backend      string // "cuda", "cpu", "vulkan", "metal"
    Quantization string
    MMProj       string
}

func DetectHardware() (*HardwareProfile, error) {
    // gopsutil/mem para RAM
    // nvidia-smi para VRAM y driver
    // gopsutil/disk para disco disponible
    // vulkaninfo para Vulkan (opcional)
}

func RecommendBackend(hw *HardwareProfile) string {
    // nvidia → cuda, apple → metal, etc.
}

func RecommendQuantization(hw *HardwareProfile) string {
    // tabla de decisión basada en RAM/VRAM
}

func DownloadLlamaServer(backend, destDir string) error {
    // Mapear backend → asset name en GitHub releases
    // Ej: "cuda" → "llama-bXXXX-bin-win-cuda-x64.zip"
    // Descargar + extraer solo llama-server.exe
}
```

---

## Quantizations disponibles (Qwen3.5-4B-GGUF)

Del repositorio `unsloth/Qwen3.5-4B-GGUF`:

| Archivo GGUF | Tamaño | Perfil |
|-------------|--------|--------|
| `Qwen3.5-4B-Q3_K_S.gguf` | 2.11 GB | Mínimo |
| `Qwen3.5-4B-Q3_K_M.gguf` | 2.29 GB | Bajo |
| `Qwen3.5-4B-Q4_K_S.gguf` | 2.59 GB | Medio-bajo |
| `Qwen3.5-4B-Q4_K_M.gguf` | 2.74 GB | **Recomendado** |
| `Qwen3.5-4B-Q5_K_M.gguf` | 3.14 GB | Alta calidad |
| `Qwen3.5-4B-Q6_K.gguf` | 3.53 GB | Alta calidad+ |
| `Qwen3.5-4B-Q8_0.gguf` | 4.48 GB | Calidad máxima |

Multimodal projector:
| Archivo mmproj | Tamaño |
|----------------|--------|
| `mmproj-F16.gguf` | 676 MB |
| `mmproj-BF16.gguf` | 676 MB |

---

## Config file

**Ubicación**: `~/.go-vision-mcp/config.json` (Windows: `%USERPROFILE%\.go-vision-mcp\config.json`)

Generado automáticamente en la primera ejecución o con `--configure`:

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
  "download_mirror": "https://github.com/ggml-org/llama.cpp/releases",
  "custom_prompt": "Analyze this image and respond to: %s"
}
```

Campos clave:
- `llama_backend`: "cuda" | "cpu" | "vulkan" | "metal" — determina qué binario de llama-server descargar/usar
- `llama_bin`: nombre del binario (difiere por SO + backend)
- `download_mirror`: fuente de descarga de llama-server (útil para mirrors o versiones custom)
- `quantization`: el usuario puede cambiarlo en cualquier momento; al reiniciar se descarga el nuevo archivo si no existe

Si el usuario cambia `repo_id`, el MCP server descarga modelos de ese repo automáticamente. Compatible con cualquier modelo GGUF multimodal de HuggingFace.

---

## Herramientas MCP

### Tool 1: `analyze_image`

```json
{
  "name": "analyze_image",
  "description": "Analyze an image with a custom prompt using the local vision model.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "prompt": { "type": "string", "description": "Question or instruction about the image" },
      "image": { "type": "string", "description": "URL (http/https), local file path, or base64 data URI" }
    },
    "required": ["prompt", "image"]
  }
}
```

### Tool 2: `describe_image`

```json
{
  "name": "describe_image",
  "description": "Get a visual description of an image.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "image": { "type": "string", "description": "URL, file path, or data URI" },
      "detail": { "type": "string", "enum": ["brief", "detailed"], "default": "detailed" }
    },
    "required": ["image"]
  }
}
```

---

## Interfaz por CLI

| Flag | Descripción |
|------|-------------|
| (ninguno) | Inicia el MCP server en modo STDIO (default para MCP) |
| `--configure` | Abre el **TUI interactivo** paso a paso (hardware, backend, quant, instalar) |
| `--install` | Instalación rápida no-interactiva (usa defaults detectados) |
| `--uninstall` | Remueve `~/.go-vision-mcp/` y limpia el PATH |
| `--status` | Muestra estado: hardware, modelo, config |
| `--download` | Descarga/verifica modelos sin iniciar server |
| `--generate-agent-config` | Genera un archivo `.md` con instrucciones para configurar el MCP en el agente del usuario |
| `--version` | Muestra versión |

---

## TUI de instalación (Bubble Tea)

El wizard de instalación (`--configure`) usa **`charmbracelet/bubbletea`** para una experiencia interactiva con componentes visuales:

| Pantalla | Componente Bubble Tea | Descripción |
|----------|----------------------|-------------|
| Hardware detectado | `lipgloss` styled box | Muestra RAM, VRAM, GPU, disco en una tabla formateada con colores |
| Selección de backend | `bubbles/list` | Lista de backends: CUDA, CPU, Vulkan, Metal. El recomendado aparece marcado |
| Selección de quantization | `bubbles/list` | Lista de quantizaciones con tamaño y badge "RECOMENDADO" |
| Instalación PATH | `bubbles/yesno` | Diálogo sí/no estilizado |
| Descarga de modelos | `bubbles/progress` | Barra de progreso animada en tiempo real |
| Resumen final | `lipgloss` styled box | Checkmarks verdes, instrucciones de configuración |
| Generar agent config | `bubbles/yesno` | "¿Generar archivo de configuración para tu agente?" |

### Mockup del TUI:

```
┌─────────────────────────────────────────────────┐
│  Vision MCP ─ Setup Wizard                      │
│  Paso 3 de 6: Seleccionar cuantización          │
│                                                  │
│  VRAM detectada: 8.0 GB                         │
│  RAM disponible: 12.5 GB                        │
│                                                  │
│  ┌─────────────────────────────────────────┐    │
│  │ ○ Q5_K_M  3.14 GB  Alta calidad   ★    │    │
│  │ ● Q4_K_M  2.74 GB  Balanceado           │    │
│  │ ○ Q3_K_M  2.29 GB  Económico            │    │
│  │ ○ Q8_0    4.48 GB  Máxima calidad       │    │
│  └─────────────────────────────────────────┘    │
│                                                  │
│  [Enter] confirmar    [/] buscar    [q] salir    │
└─────────────────────────────────────────────────┘
```

```go
func runWizard() (*InstallSelection, error) {
    // Modelo Bubble Tea con 6 screens
    // 1. hardwareDetectScreen
    // 2. backendSelectScreen
    // 3. quantSelectScreen
    // 4. installPathScreen
    // 5. downloadProgressScreen (modelo asíncrono)
    // 6. summaryScreen
    // Cada paso = un modelo Bubble Tea con su Update/View
}
```

### Instalación portable (sin `--install`):

Si el usuario no quiere instalar, el MCP server funciona igual desde cualquier carpeta. La config se guarda en el directorio actual como `vision-mcp.json`.

---

## Generación de archivo de configuración para el agente (`--generate-agent-config`)

El usuario puede generar un archivo `.md` con instrucciones precisas para que su agente (Kilo Code, OpenCode, PI Agent, etc.) configure el MCP server automáticamente.

### Contenido generado:

```markdown
# Instrucciones para configurar Vision MCP en tu agente

Este archivo fue generado por Vision MCP Setup Wizard.
Pásalo a tu agente para que configure el MCP server automáticamente.

## Configuración del MCP Server

```json
{
  "mcpServers": {
    "vision": {
      "command": "C:\\Users\\tu\\.go-vision-mcp\\vision-mcp.cmd",
      "env": {
        "VISION_MCP_PORT": "8001",
        "VISION_MCP_LLAMA_BACKEND": "cuda"
      }
    }
  }
}
```

## Tools disponibles

| Tool | Descripción |
|------|-------------|
| `analyze_image(prompt, image)` | Analiza una imagen con un prompt personalizado |
| `describe_image(image)` | Describe una imagen en detalle |

## Dónde configurar según tu agente

### Kilo Code
- Archivo: `/path/to/.kilocode/mcp.json`
- Pegar el JSON de configuración arriba

### OpenCode
- Archivo: `~/.opencode/mcp.json`
- O configurar vía interfaz de OpenCode

### PI Agent
- Archivo de configuración del proyecto
- Agregar al bloque `mcpServers`

### Claude Desktop
- Archivo: `claude_desktop_config.json`
- Ubicación: `%APPDATA%\Claude\claude_desktop_config.json`

## Verificar instalación

```bash
vision-mcp --status
```
```
```

El agente lee este archivo y configura automáticamente el `mcp.json` correspondiente.

### Implementación:

```go
type AgentConfig struct {
    InstallPath string   // ruta donde está instalado vision-mcp
    Clients     []string // lista de clientes detectados/instalados
}

func GenerateAgentConfig(path string) error {
    // Template con instrucciones + JSON config
    // Detectar qué clientes tiene el usuario:
    // - ¿Existe ~/.opencode/mcp.json?
    // - ¿Existe %APPDATA%/Claude/claude_desktop_config.json?
    // - ¿Existe .vscode/mcp.json en proyectos?
    // Incluir config lista para copiar+pegar
    // Guardar en path (ej: ~/Desktop/vision-mcp-agent-setup.md)
}
```

Como parte del wizard, en el paso final se pregunta:

```
  ┌─────────────────────────────────────────────┐
  │  ¿Generar archivo de configuración           │
  │  para tu agente?                             │
  │                                             │
  │  Esto creará un .md con instrucciones        │
  │  listas para que tu agente configure         │
  │  el MCP server automáticamente.             │
  │                                             │
  │  [Sí, generar]  [No, ya sé cómo hacerlo]    │
  └─────────────────────────────────────────────┘

  ✓ Archivo generado: C:\Users\tu\Desktop\vision-mcp-setup.md
    Arrástralo a tu agente o pásalo con @archivo
```

---

## PATH management

### Windows (`--install`):

```go
func ensureInPATH(installDir string) error {
    // Leer PATH de usuario de HKCU\Environment
    // Si installDir no est� en PATH, agregarlo al final
    // Escribir de vuelta con reg.exe o powershell
    // Broadcast WM_SETTINGCHANGE para aplicar cambios
}
```

### Linux/macOS (`--install`):

```go
func ensureInPATH(installDir string) error {
    // Detectar shell: bash, zsh, fish
    // Agregar 'export PATH="$PATH:installDir"' a ~/.bashrc / ~/.zshrc
}
```

### `--uninstall`:

Inverso: remover la carpeta del PATH y borrar `~/.go-vision-mcp/`.

---

## Estructura del proyecto

```
vision-mcp/
├── go.mod
├── go.sum
├── Makefile                     # build, test, run, lint
├── main.go                      # Entry point: CLI flags + dispatcher
├── cmd/
│   └── vision-mcp/
│       └── main.go              # Binario principal
├── internal/
│   ├── config/
│   │   ├── config.go            # Carga/guarda config.json
│   │   ├── defaults.go          # Valores por defecto + auto-detect
│   │   └── config_test.go
│   ├── hardware/
│   │   ├── detect.go            # RAM, VRAM, disco
│   │   ├── recommend.go         # quantization seg�n hardware
│   │   └── detect_test.go
│   ├── download/
│   │   ├── download.go          # Descarga modelos GGUF desde HF
│   │   ├── llamabin.go          # Descarga + extracción de llama-server desde GitHub releases
│   │   ├── progress.go          # Barra de progreso en terminal
│   │   └── download_test.go
│   ├── llamaserver/
│   │   ├── server.go            # Spawn + health check + stop
│   │   └── server_test.go       # Mock HTTP server para tests
│   ├── image/
│   │   ├── resolve.go           # URL/path/base64 → data URI
│   │   └── resolve_test.go
│   ├── mcp/
│   │   ├── tools.go             # Definici�n y handlers de tools
│   │   └── tools_test.go
│   ├── installer/
│   │   ├── install.go           # Copiar binario, PATH, launcher
│   │   ├── uninstall.go
│   │   ├── readme.go            # Generar README.md en dir de instalaci�n
│   │   └── install_test.go
│   ├── setup/
│   │   ├── wizard.go            # Orquestador del TUI wizard
│   │   ├── screens.go           # Pantallas del TUI (bubbletea models)
│   │   └── styles.go            # Estilos lipgloss
│   └── agentconfig/
│       └── generate.go          # Generar .md para configurar el agente
├── models/                      # GGUF descargados (gitignored)
│   └── .gitkeep
└── docs/
    ├── README.md                # Documentaci�n principal
    └── ARCHITECTURE.md          # Dise�o t�cnico detallado
```

---

## Tests

### Unit tests (en cada paquete):

| Paquete | Qu� probar |
|---------|-----------|
| `config` | Parse/save de config, valores default, migraci�n |
| `hardware` | Detecci�n de RAM/VRAM, recomendaci�n de quant |
| `download` | Descarga con progreso, errores HTTP, checksum |
| `image` | URL → base64, path → base64, data URI passthrough, tipos MIME |
| `llamaserver` | Spawn, health check, timeout, graceful shutdown |
| `mcp/tools` | Schemas de tools, validaci�n de inputs |
| `installer` | PATH management, creaci�n de directorios, launcher |

### Integration tests:

- `TestAnalyzeImageWithMockServer`: spin up un mock HTTP server que responde como llama-server, verificar que el tool handler parsea la respuesta correctamente
- `TestFullPipeline`: download + spawn + analyze (opcional, requiere llama-server real)
- `TestConfigPersistence`: guardar config, reiniciar, verificar que se carga

### Ejecución:

```bash
make test           # unit tests
make test-integ     # integration tests (requiere llama-server en PATH)
make test-all       # ambos
```

---

## Documentación

### `docs/README.md`:

- Qué es Vision MCP
- Requisitos del sistema
- Instalación rápida (`--install`)
- Configuración (`config.json`)
- Herramientas disponibles (`analyze_image`, `describe_image`)
- Cómo usar con:
  - Claude Desktop (mcp.json)
  - Cline / Roo Code
  - Cualquier cliente MCP
- Solución de problemas (llama-server no encontrado, CUDA, etc.)
- Cambiar de modelo (configurar repo_id diferente)

### `docs/ARCHITECTURE.md`: Arquitectura y decisiones técnicas

### README generado en el directorio de instalación (`~/.go-vision-mcp/README.md`):

Cuando se ejecuta `--install`, el sistema crea automáticamente un `README.md` en el directorio de instalación. Este README incluye configuraciones listas para copiar y pegar para cada cliente MCP:

```markdown
# Vision MCP - Instalado en esta carpeta

## Cómo usar con tu cliente MCP favorito

### OpenCode
Crea o edita `~/.opencode/mcp.json`:
```json
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
```

### PI Agent
En la configuración del proyecto o global, agrega:
```json
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
```

### Kilo Code
Agrega a tu configuración MCP:
```json
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
```

### Claude Desktop
Edita `claude_desktop_config.json`:
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
En `.vscode/mcp.json` o settings:
```json
{
  "mcpServers": {
    "vision": {
      "command": "vision-mcp"
    }
  }
}
```

### Manual (terminal)
```bash
vision-mcp
```
El servidor se inicia en modo STDIO, esperando conexión del cliente.

---

## Configuración

Edita `config.json` en esta misma carpeta para cambiar modelo,
cuantización, puerto, etc.

### Cambiar de modelo
```json
{
  "repo_id": "otro-usuario/otro-modelo-GGUF",
  "quantization": "Q4_K_M",
  "mmproj": "mmproj-F16.gguf"
}
```

### Solución de problemas

| Problema | Solución |
|----------|----------|
| "llama-server not found" | Descargar llama-server desde https://github.com/ggml-org/llama.cpp/releases y ponerlo en esta carpeta o en el PATH |
| "CUDA error" | Asegurar que los drivers NVIDIA están instalados. Ejecutar `nvidia-smi` para verificar. |
| "Out of memory" | Usar una cuantización más baja (Q3_K_M) en config.json |
| "Model not downloading" | Verificar conexión a internet. El archivo se descarga desde HuggingFace. |
```

Contenido generado dinámicamente: las rutas y nombres de archivo reflejan la instalación real del usuario.

  ### `docs/ARCHITECTURE.md`:

  - Diagrama de componentes y flujo de datos
  - Decisión: sidecar vs binding directo
  - Formato de comunicación con llama-server
  - Manejo de errores y edge cases

  ---

## Plan de implementación (fases)

### Fase 1: Core MVP

- [ ] Init Go module + dependencias (`mark3labs/mcp-go`, `gopsutil/v3`)
- [ ] `hardware/detect.go` — RAM, VRAM, disco. `hardware/recommend.go` — elegir quantization
- [ ] `config/config.go` — load/save JSON, defaults desde hardware detection
- [ ] `download/download.go` — descargar GGUF + mmproj desde HF con barra de progreso
- [ ] `llamaserver/server.go` — spawn + health check + graceful shutdown
- [ ] `image/resolve.go` — URL/path → data URI base64
- [ ] `mcp/tools.go` — tool `analyze_image`, conexión a llama-server vía HTTP
- [ ] `main.go` — CLI flags, init flow, MCP server loop
- [ ] `Makefile` — build, test, run
- [ ] Test con MCP Inspector

### Fase 2: Installer + PATH + wizard

- [ ] `download/llamabin.go` — detectar y descargar llama-server según backend (CUDA/CPU/Vulkan)
- [ ] `installer/install.go` — copiar binario, crear launcher `.cmd`, PATH management
- [ ] `installer/uninstall.go` — remover directorio, limpiar PATH
- [ ] `installer/readme.go` — generar README.md en el directorio de instalación
- [ ] `setup/wizard.go` — orquestador del TUI con bubbletea (6 pantallas)
- [ ] `setup/screens.go` — cada pantalla del wizard como un model bubbletea
- [ ] `setup/styles.go` — estilos lipgloss para el TUI
- [ ] `agentconfig/generate.go` — generar .md con instrucciones para el agente del usuario
- [ ] `main.go` — flags `--install`, `--uninstall`, `--configure`, `--status`, `--generate-agent-config`
- [ ] Launcher script `vision-mcp.cmd` para Windows
- [ ] Manejar edge cases: PATH corrupto, permisos, antivirus

### Fase 3: Tools completas + tests

- [ ] Tool `describe_image` con prompt predefinido
- [ ] Soporte para múltiples imágenes en un prompt
- [ ] Unit tests para todos los paquetes
- [ ] Integration test con mock llama-server
- [ ] CI: GitHub Actions (lint + test)

### Fase 4: Documentación y polish

- [ ] `docs/README.md` completo
- [ ] `docs/ARCHITECTURE.md`
- [ ] README generado dinámicamente en `~/.go-vision-mcp/README.md` con configs por cliente
- [ ] Manejo de errores robusto (llama-server crash, timeout, imagen inválida)
- [ ] Logs estructurados (JSON o texto plano)
- [ ] Verificar comportamiento en Windows, macOS y Linux
- [ ] Release binario compilado por plataforma/backend

---

## Limitaciones

| Aspecto | Nota |
|---------|------|
| **Velocidad** | ~5-30s por imagen en CPU; ~1-3s con GPU (CUDA) |
| **RAM** | ~4-6 GB (modelo + mmproj + KV cache) |
| **Dependencia externa** | Requiere `llama-server.exe` en PATH o en `~/.go-vision-mcp/` |
| **Download inicial** | ~2.9-3.8 GB según quantization |
| **Primera carga** | llama-server tarda ~10-30s en cargar el modelo a GPU |
| **GPU** | Solo NVIDIA CUDA detectado automáticamente; Metal/Vulkan manual |
