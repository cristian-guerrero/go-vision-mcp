package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/cristian-guerrero/go-vision-mcp/internal/clipboard"
	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
	"github.com/cristian-guerrero/go-vision-mcp/internal/download"
	"github.com/cristian-guerrero/go-vision-mcp/internal/hardware"
	"github.com/cristian-guerrero/go-vision-mcp/internal/llamaserver"
	mcptools "github.com/cristian-guerrero/go-vision-mcp/internal/mcp"
)

// ServerManager orchestrates the MCP server lifecycle: async
// initialization (hardware detection, port reuse), lazy model download
// and llama-server startup, idle timeout monitoring, and signal
// handling. It encapsulates the closure-based lifecycle that was
// previously inline in runServer(), making the logic testable and
// ensuring thread-safe access to the shared llama-srv reference.
type ServerManager struct {
	cfg     *config.Config
	handler *mcptools.ToolHandler
	clipMon clipboard.MonitorInterface

	mu       sync.Mutex
	llamaSrv llamaserver.ServerInterface

	lock        *llamaserver.Lock
	assetsReady chan struct{}
}

// NewServerManager creates a ServerManager that owns the server-side
// lifecycle. It does not start any goroutines; call Run() for that.
func NewServerManager(cfg *config.Config, handler *mcptools.ToolHandler, clipMon clipboard.MonitorInterface) *ServerManager {
	return &ServerManager{
		cfg:     cfg,
		handler: handler,
		clipMon: clipMon,
		lock:    llamaserver.NewLock(),
	}
}

// Run starts the MCP server in STDIO mode. It performs async
// initialization (hardware detection, port reuse check, asset
// download) in background goroutines, while the main goroutine blocks
// on ServeStdio. When the MCP client disconnects, cleanup runs.
func (sm *ServerManager) Run(mcpServer *server.MCPServer) {
	sm.assetsReady = make(chan struct{})

	sm.handler.SetStopFunc(sm.stop)
	sm.handler.SetRestartFunc(sm.restartLlama)

	go sm.initAsync()

	if sm.cfg.IdleTimeout > 0 {
		go sm.idleMonitor()
	}

	go sm.signalHandler()

	log.Printf("MCP server ready (STDIO mode)")
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Printf("MCP server error: %v", err)
	}
}

// initAsync runs once on startup: detects hardware, checks for an
// existing llama-server on the configured port, and optionally starts
// a background asset download. It signals readiness to the handler
// when the initial setup is complete.
func (sm *ServerManager) initAsync() {
	hw, err := hardware.DetectHardware()
	if err == nil {
		log.Printf("Hardware: RAM=%dGB VRAM=%dGB",
			hw.TotalRAM/(1024*1024*1024),
			hw.GPU.VRAM/(1024*1024*1024))

		if sm.cfg.LlamaBackend == "" || sm.cfg.LlamaBackend == "cuda" && !hw.GPU.Present {
			sm.cfg.LlamaBackend = hardware.RecommendBackend(hw)
		}
		if sm.cfg.LlamaBackend == "cpu" {
			sm.cfg.NGL = 0
		}
		sm.cfg.Save()
	}

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", sm.cfg.Port)
	hc := &http.Client{Timeout: 2 * time.Second}
	alreadyRunning := false

	if resp, err := hc.Get(healthURL); err == nil {
		resp.Body.Close()
		log.Printf("llama-server already running on port %d, reusing", sm.cfg.Port)
		sm.handler.SetLlamaURL(fmt.Sprintf("http://127.0.0.1:%d", sm.cfg.Port))
		sm.handler.SetLoaded(true)
		alreadyRunning = true
		if err := sm.lock.AddPID(); err != nil {
			log.Printf("Warning: failed to register in lock: %v", err)
		}
	}

	if !alreadyRunning && sm.cfg.AutoDownload {
		go sm.downloadAssets()
	} else {
		close(sm.assetsReady)
	}

	sm.handler.SetReady()
}

// stop tears down the clipboard monitor (if active) and llama-server.
// The lock file determines whether this process should actually stop
// llama-server or keep it alive for other MCP processes.
func (sm *ServerManager) stop() {
	sm.handler.SetLoaded(false)

	if sm.clipMon != nil {
		sm.clipMon.Stop()
	}

	shouldStop, _ := sm.lock.Release()

	sm.mu.Lock()
	srv := sm.llamaSrv
	sm.mu.Unlock()

	if shouldStop && srv != nil {
		srv.Stop()
	} else {
		log.Printf("llama-server kept alive for other MCP processes")
	}
}

// restartLlama downloads assets (if needed), starts llama-server, and
// registers the new instance. It blocks until assets are ready and the
// server passes its health check. Called on-demand from chatCompletion.
func (sm *ServerManager) restartLlama(ctx context.Context) error {
	select {
	case <-sm.assetsReady:
	case <-ctx.Done():
		return ctx.Err()
	}

	if sm.cfg.AutoDownload {
		if err := download.EnsureModels(sm.cfg, downloadProgress("Model")); err != nil {
			return fmt.Errorf("download models: %w", err)
		}
	}

	llamaBin, newLlamaPath, err := resolveLlamaServer(sm.cfg)
	if err != nil {
		return fmt.Errorf("resolve llama-server: %w", err)
	}
	if newLlamaPath != "" {
		sm.cfg.LlamaServerPath = newLlamaPath
		sm.cfg.Save()
	}

	newSrv := llamaserver.New(llamaserver.Options{
		ModelPath:    sm.cfg.ModelPath(),
		MMProjPath:   sm.cfg.MMProjPath(),
		Port:         sm.cfg.Port,
		NGL:          sm.cfg.NGL,
		NCtx:         sm.cfg.NCtx,
		FlashAttn:    sm.cfg.FlashAttn,
		BinaryName:   llamaBin,
		KvCacheTypeK: sm.cfg.KvCacheTypeK,
		KvCacheTypeV: sm.cfg.KvCacheTypeV,
	})
	if err := newSrv.Start(ctx); err != nil {
		return fmt.Errorf("start llama-server: %w", err)
	}

	sm.mu.Lock()
	sm.llamaSrv = newSrv
	sm.mu.Unlock()

	sm.lock.Start(sm.cfg.Port)
	sm.handler.SetLlamaURL(newSrv.URL())
	sm.handler.SetLoaded(true)
	return nil
}

// idleMonitor runs a ticker that checks for inactivity every 30
// seconds. When the handler reports no tool calls within the idle
// timeout window, llama-server is stopped to free GPU memory.
func (sm *ServerManager) idleMonitor() {
	idleDuration := time.Duration(sm.cfg.IdleTimeout) * time.Minute
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !sm.handler.IsLoaded() {
			continue
		}
		if sm.handler.IdleTime() > idleDuration {
			log.Printf("Idle timeout (%d min), freeing GPU memory", sm.cfg.IdleTimeout)
			sm.handler.SetLoaded(false)
			sm.lock.ForceClear()

			sm.mu.Lock()
			srv := sm.llamaSrv
			sm.llamaSrv = nil
			sm.mu.Unlock()

			if srv != nil {
				srv.Stop()
			} else {
				pid := llamaserver.FindProcessOnPort(sm.cfg.Port)
				if pid > 0 {
					proc, err := os.FindProcess(pid)
					if err == nil {
						proc.Signal(syscall.SIGTERM)
						time.Sleep(2 * time.Second)
						proc.Kill()
					}
				}
			}
		}
	}
}

// signalHandler blocks on SIGINT/SIGTERM and performs a graceful
// shutdown when either signal is received.
func (sm *ServerManager) signalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Printf("Shutting down...")
	sm.handler.Stop()
	os.Exit(0)
}

// downloadAssets runs model download and llama-server binary download
// in the background. It closes assetsReady when done (or on error).
func (sm *ServerManager) downloadAssets() {
	downloadErr := func() error {
		if err := download.EnsureModels(sm.cfg, downloadProgress("Model")); err != nil {
			return err
		}
		_, newLlamaPath, err := resolveLlamaServer(sm.cfg)
		if err != nil {
			return err
		}
		if newLlamaPath != "" {
			sm.cfg.LlamaServerPath = newLlamaPath
		}
		sm.cfg.ModelPathOverride = sm.cfg.ModelPath()
		sm.cfg.MMProjPathOverride = sm.cfg.MMProjPath()
		sm.cfg.Save()
		return nil
	}()
	if downloadErr != nil {
		log.Printf("Warning: background asset download failed: %v", downloadErr)
	}
	close(sm.assetsReady)
}
