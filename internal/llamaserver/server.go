// Package llamaserver manages the llama-server sidecar process.
// It starts the binary as a subprocess with flags for KV cache control
// and idle VRAM offloading (--sleep-idle-seconds), waits for its health
// endpoint, and provides graceful shutdown with SIGTERM → 3s → SIGKILL.
package llamaserver

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// Options holds all configuration for a llama-server subprocess.
// Fields map directly to llama-server CLI arguments.
type Options struct {
	ModelPath    string
	MMProjPath   string
	Port         int
	NGL          int
	NCtx         int
	FlashAttn    bool
	BinaryName   string
	KvCacheTypeK string
	KvCacheTypeV string
	// IdleTimeout is the number of idle minutes before llama-server
	// unloads the model from VRAM (passed as --sleep-idle-seconds * 60).
	// Zero disables automatic model unloading.
	IdleTimeout int
}

// Server wraps a llama-server subprocess. It holds the command,
// arguments, and health-check state.
type Server struct {
	cmd         *exec.Cmd
	port        int
	mmproj      string
	model       string
	ngl         int
	nctx        int
	flash       bool
	bin         string
	ctk         string
	ctv         string
	idleTimeout int
}

// New creates a Server but does not start it.
func New(opts Options) *Server {
	return &Server{
		model:       opts.ModelPath,
		mmproj:      opts.MMProjPath,
		port:        opts.Port,
		ngl:         opts.NGL,
		nctx:        opts.NCtx,
		flash:       opts.FlashAttn,
		bin:         opts.BinaryName,
		ctk:         opts.KvCacheTypeK,
		ctv:         opts.KvCacheTypeV,
		idleTimeout: opts.IdleTimeout,
	}
}

// Start launches llama-server as a subprocess with the configured
// arguments (model, mmproj, port, GPU layers, context size, flash
// attention, KV cache quantization, -cram 0, and --sleep-idle-seconds
// when idle_timeout > 0). It then polls GET /health up to 60 seconds
// until the server responds 200 OK.
func (s *Server) Start(ctx context.Context) error {
	binary := s.bin
	if binary == "" {
		binary = defaultBinary()
	}

	if _, err := os.Stat(s.model); os.IsNotExist(err) {
		return fmt.Errorf("model file not found: %s", s.model)
	}
	if _, err := os.Stat(s.mmproj); os.IsNotExist(err) {
		return fmt.Errorf("mmproj file not found: %s", s.mmproj)
	}

	ctk := s.ctk
	if ctk == "" {
		ctk = "q4_0"
	}
	ctv := s.ctv
	if ctv == "" {
		ctv = "q4_0"
	}

	args := []string{
		"-m", s.model,
		"--mmproj", s.mmproj,
		"--port", fmt.Sprintf("%d", s.port),
		"--n-gpu-layers", fmt.Sprintf("%d", s.ngl),
		"--ctx-size", fmt.Sprintf("%d", s.nctx),
		"--host", "127.0.0.1",
		"-ctk", ctk,
		"-ctv", ctv,
	}
	if s.flash {
		args = append(args, "-fa", "on")
	}
	args = append(args, "--jinja")
	args = append(args, "--reasoning", "off")
	args = append(args, "--no-webui")
	args = append(args, "-cram", "0")
	if s.idleTimeout > 0 {
		args = append(args, "--sleep-idle-seconds", fmt.Sprintf("%d", s.idleTimeout*60))
	}

	log.Printf("Executing: %s %v", binary, args)

	s.cmd = exec.CommandContext(ctx, binary, args...)
	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[llama-server] %s", scanner.Text())
		}
	}()

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", binary, err)
	}

	return s.waitForHealth(ctx, 60*time.Second)
}

// waitForHealth polls the llama-server health endpoint every 2 seconds
// until it responds with 200 OK or the context/timeout expires.
func (s *Server) waitForHealth(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", s.port)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		resp, err := client.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("llama-server health check timeout after %v", timeout)
		case <-time.After(2 * time.Second):
		}
	}
}

// Stop gracefully terminates llama-server: sends SIGTERM, waits up to
// 3 seconds, then escalates to SIGKILL.
func (s *Server) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return s.cmd.Process.Kill()
	}

	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-time.After(3 * time.Second):
		return s.cmd.Process.Kill()
	case err := <-done:
		return err
	}
}

// URL returns the base URL of the running server, e.g.
// http://127.0.0.1:8001.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func defaultBinary() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}
