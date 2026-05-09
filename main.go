package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/vision-mcp/internal/agentconfig"
	"github.com/vision-mcp/internal/config"
	"github.com/vision-mcp/internal/download"
	"github.com/vision-mcp/internal/hardware"
	"github.com/vision-mcp/internal/installer"
	"github.com/vision-mcp/internal/llamaserver"
	mcptools "github.com/vision-mcp/internal/mcp"
	"github.com/vision-mcp/internal/setup"
)

const version = "1.0.0"

func main() {
	runConfigure := flag.Bool("configure", false, "Open interactive TUI wizard")
	runInstall := flag.Bool("install", false, "Quick non-interactive install")
	runUninstall := flag.Bool("uninstall", false, "Remove installation")
	showStatus := flag.Bool("status", false, "Show status")
	downloadOnly := flag.Bool("download", false, "Download/verify models")
	generateAgent := flag.String("generate-agent-config", "", "Generate agent config file (optional output path)")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Println("vision-mcp v" + version)
		return
	}

	if *showStatus {
		displayStatus()
		return
	}

	if *downloadOnly {
		runDownload()
		return
	}

	if *runUninstall {
		runUninstallCmd()
		return
	}

	if *generateAgent != "" || (len(os.Args) > 1 && os.Args[1] == "--generate-agent-config") {
		path := *generateAgent
		if path == "" && len(os.Args) > 2 && os.Args[1] == "--generate-agent-config" {
			path = os.Args[2]
		}
		fmt.Printf("Generating agent config...\n")
		if err := agentconfig.Generate(path); err != nil {
			log.Fatalf("Error: %v", err)
		}
		if path == "" {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, "Desktop", "vision-mcp-setup.md")
		}
		fmt.Printf("Agent config generated: %s\n", path)
		return
	}

	if *runConfigure {
		runWizardCmd()
		return
	}

	if *runInstall {
		runInstallCmd()
		return
	}

	runServer()
}

func runWizardCmd() {
	cfg, err := setup.RunWizard()
	if err != nil {
		log.Fatalf("Wizard error: %v", err)
	}
	if cfg != nil {
		if err := cfg.Save(); err != nil {
			log.Fatalf("Error saving config: %v", err)
		}
		fmt.Printf("\nConfiguration saved to %s\n", config.ConfigPath())
		fmt.Printf("Run 'vision-mcp' to start the server.\n")
	}
}

func runInstallCmd() {
	installDir := installer.InstallDir()
	fmt.Printf("Installing to %s...\n", installDir)

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("Error finding executable: %v", err)
	}

	if err := installer.Install(installDir, exe); err != nil {
		log.Fatalf("Install error: %v", err)
	}

	if err := installer.GenerateReadme(installDir); err != nil {
		fmt.Printf("Warning: Could not generate README: %v\n", err)
	}

	cfg := config.DefaultConfig()
	hw, err := hardware.DetectHardware()
	if err == nil {
		cfg.Quantization = hardware.RecommendQuantization(hw)
		cfg.LlamaBackend = hardware.RecommendBackend(hw)
	}
	cfg.Save()

	fmt.Println("Installation complete!")
	fmt.Printf("Config saved to %s\n", config.ConfigPath())
	fmt.Printf("Models will download on first server start.\n")
	fmt.Printf("Run 'vision-mcp' to start.\n")
}

func runUninstallCmd() {
	if err := installer.Uninstall(); err != nil {
		log.Fatalf("Uninstall error: %v", err)
	}
}

func runServer() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	hw, err := hardware.DetectHardware()
	if err == nil {
		log.Printf("Hardware: RAM=%dGB VRAM=%dGB",
			hw.TotalRAM/(1024*1024*1024),
			hw.GPU.VRAM/(1024*1024*1024))

		backend := hardware.RecommendBackend(hw)
		quant := hardware.RecommendQuantization(hw)

		if cfg.LlamaBackend == "" || cfg.LlamaBackend == "cuda" && !hw.GPU.Present {
			cfg.LlamaBackend = backend
		}
		if cfg.Quantization == "Q4_K_M" {
			cfg.Quantization = quant
		}
		cfg.Save()
	}

	if cfg.AutoDownload {
		log.Printf("Checking models...")
		if err := download.EnsureModels(cfg, nil); err != nil {
			log.Fatalf("Error downloading models: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("Starting llama-server on port %d...", cfg.Port)
	srv := llamaserver.New(
		cfg.ModelPath(),
		cfg.MMProjPath(),
		cfg.Port,
		cfg.NGL,
		cfg.NCtx,
		cfg.FlashAttn,
		cfg.LlamaBin,
	)
	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Error starting llama-server: %v", err)
	}
	log.Printf("llama-server ready")

	defer srv.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("Shutting down...")
		cancel()
		srv.Stop()
		os.Exit(0)
	}()

	mcpServer := server.NewMCPServer("vision-mcp", "1.0.0")
	handler := mcptools.NewToolHandler(srv.URL(), cfg.CustomPrompt)
	handler.RegisterTools(mcpServer)

	log.Printf("MCP server ready (STDIO mode)")
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}

func displayStatus() {
	cfg, _ := config.LoadConfig()
	fmt.Println("Vision MCP Status")
	fmt.Println("=================")
	fmt.Printf("Config:       %s\n", config.ConfigPath())
	fmt.Printf("Quantization: %s\n", cfg.Quantization)
	fmt.Printf("MMProj:       %s\n", cfg.MMProj)
	fmt.Printf("Backend:      %s\n", cfg.LlamaBackend)
	fmt.Printf("Port:         %d\n", cfg.Port)
	fmt.Printf("Model path:   %s\n", cfg.ModelPath())
	fmt.Printf("MMProj path:  %s\n", cfg.MMProjPath())

	hw, err := hardware.DetectHardware()
	if err == nil {
		fmt.Println()
		fmt.Println("Hardware:")
		fmt.Printf("  RAM:  %.1f GB (available: %.1f GB)\n",
			float64(hw.TotalRAM)/(1024*1024*1024),
			float64(hw.AvailableRAM)/(1024*1024*1024))
		if hw.GPU.Present {
			fmt.Printf("  GPU:  %s (VRAM: %.1f GB, driver: %s)\n",
				hw.GPU.Vendor,
				float64(hw.GPU.VRAM)/(1024*1024*1024),
				hw.GPU.DriverVer)
		} else {
			fmt.Println("  GPU:  none (CPU only)")
		}
		fmt.Printf("  Disk: %.1f GB free\n", float64(hw.FreeDisk)/(1024*1024*1024))
		fmt.Printf("  Recommended backend: %s\n", hardware.RecommendBackend(hw))
		fmt.Printf("  Recommended quantization: %s\n", hardware.RecommendQuantization(hw))
	}

	fmt.Println()
	fmt.Println("Tools:")
	fmt.Println("  analyze_image(prompt, image)")
	fmt.Println("  describe_image(image, detail)")
}

func runDownload() {
	cfg, _ := config.LoadConfig()
	fmt.Println("Downloading models...")
	if err := download.EnsureModels(cfg, downloadProgress); err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Println("\nModels ready.")
}

func downloadProgress(downloaded, total int64) {
	if total > 0 && downloaded > 0 {
		pct := float64(downloaded) / float64(total) * 100
		fmt.Printf("\rDownloading... %.1f%% (%s/%s)  ",
			pct,
			download.FormatBytes(downloaded),
			download.FormatBytes(total))
	}
}
