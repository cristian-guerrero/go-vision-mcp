package llamaserver

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

type Server struct {
	cmd    *exec.Cmd
	port   int
	mmproj string
	model  string
	ngl    int
	nctx   int
	flash  bool
	bin    string
}

func New(modelPath, mmprojPath string, port, ngl, nctx int, flashAttn bool, binaryName string) *Server {
	return &Server{
		model:  modelPath,
		mmproj: mmprojPath,
		port:   port,
		ngl:    ngl,
		nctx:   nctx,
		flash:  flashAttn,
		bin:    binaryName,
	}
}

func (s *Server) Start(ctx context.Context) error {
	binary := s.bin
	if binary == "" {
		binary = defaultBinary()
	}

	args := []string{
		"-m", s.model,
		"--mmproj", s.mmproj,
		"--port", fmt.Sprintf("%d", s.port),
		"--n-gpu-layers", fmt.Sprintf("%d", s.ngl),
		"--ctx-size", fmt.Sprintf("%d", s.nctx),
		"--host", "127.0.0.1",
	}
	if s.flash {
		args = append(args, "--flash-attn")
	}

	s.cmd = exec.CommandContext(ctx, binary, args...)
	s.cmd.Stdout = nil
	s.cmd.Stderr = nil

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", binary, err)
	}

	return s.waitForHealth(ctx, 60*time.Second)
}

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

func (s *Server) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	return s.cmd.Process.Kill()
}

func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func defaultBinary() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}
