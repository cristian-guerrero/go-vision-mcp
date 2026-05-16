package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/mark3labs/mcp-go/server"

	"github.com/cristian-guerrero/go-vision-mcp/internal/agentconfig"
	"github.com/cristian-guerrero/go-vision-mcp/internal/clipboard"
	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
	"github.com/cristian-guerrero/go-vision-mcp/internal/discover"
	"github.com/cristian-guerrero/go-vision-mcp/internal/download"
	"github.com/cristian-guerrero/go-vision-mcp/internal/hardware"
	"github.com/cristian-guerrero/go-vision-mcp/internal/installer"
	"github.com/cristian-guerrero/go-vision-mcp/internal/llamaserver"
	"github.com/cristian-guerrero/go-vision-mcp/internal/logger"
	mcptools "github.com/cristian-guerrero/go-vision-mcp/internal/mcp"
	"github.com/cristian-guerrero/go-vision-mcp/internal/setup"
)

var version = "1.0.0"

func main() {
	runConfigure := flag.Bool("configure", false, "Open interactive TUI wizard")
	runInstall := flag.Bool("install", false, "Quick non-interactive install")
	runUninstall := flag.Bool("uninstall", false, "Remove installation")
	showStatus := flag.Bool("status", false, "Show status")
	downloadOnly := flag.Bool("download", false, "Download/verify models")
	generateAgent := flag.String("generate-agent-config", "", "Generate agent config file (optional output path)")
	showVersion := flag.Bool("version", false, "Show version")
	freeMemory := flag.Bool("free", false, "Free GPU memory by unloading the model")
	manualConfig := flag.Bool("manual", false, "Configure with existing models and llama-server")
	mcpSetup := flag.Bool("mcp-setup", false, "Auto-configure MCP for installed agents (Kilo Code, OpenCode, PI Agent)")
	analyzeClipboard := flag.String("analyze-clipboard", "", "Analyze the clipboard image with a custom prompt")
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

	if *freeMemory {
		runFreeMemory()
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

	if *manualConfig {
		runManualWizard()
		return
	}

	if *mcpSetup {
		runMCPSetup()
		return
	}

	if *analyzeClipboard != "" {
		runAnalyzeClipboard(*analyzeClipboard)
		return
	}

	lgr, err := logger.Init()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not initialize logger: %v\n", err)
	}
	if lgr != nil {
		defer lgr.Close()
	}

	if !isInteractive() {
		log.Printf("No interactive terminal detected.")

		if !configExists() {
			log.Printf("No config found, starting with defaults.")
		}

		runServer()
		return
	}

	if !configExists() {
		showWelcomeMenu()
		return
	}

	runServer()
}

// configExists checks whether a JSON config file exists at
// the standard path (~/.go-mcp/vision/config.json) or as a
// portable config (vision-mcp.json in the working directory).
func configExists() bool {
	if _, err := os.Stat(config.ConfigPath()); err == nil {
		return true
	}
	if _, err := os.Stat(config.PortableConfigPath()); err == nil {
		return true
	}
	return false
}

// isInteractive returns true when stdin is a character device
// (i.e., the process is running in an interactive terminal).
func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// showNonInteractiveMessage displays a one-shot message when the binary
// is double-clicked on Windows (via MessageBoxW) or prints to stderr on Unix.
func showNonInteractiveMessage() {
	msg := "Vision MCP\n\n" +
		"To use Vision MCP, run this from a terminal:\n" +
		"  vision-mcp --status\n" +
		"  vision-mcp --configure\n" +
		"  vision-mcp\n\n" +
		"Log: " + filepath.Join(config.InstallDir(), "vision-mcp.log")

	if runtime.GOOS == "windows" {
		showMessageBox("Vision MCP", msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// showWelcomeMenu displays the initial Bubble Tea TUI with 6 options:
// Quick setup, Guided wizard, Manual config, MCP setup, Status, or Exit.
func showWelcomeMenu() {
	choice := setup.RunWelcome()
	if choice == 0 {
		return
	}

	fmt.Println()

	switch choice {
	case 1:
		quickSetup()
	case 2:
		runWizardCmd()
	case 3:
		runManualWizard()
	case 4:
		runMCPSetup()
	case 5:
		displayStatus()
		fmt.Println()
		fmt.Println("Run 'vision-mcp --configure' to configure, or 'vision-mcp' to start.")
	default:
		fmt.Println("Exiting.")
	}
}

// quickSetup runs a non-interactive auto-install: copies the binary,
// detects hardware, recommends quantization/backend, and optionally
// finds existing GGUF models in the default models directory.
func quickSetup() {
	fmt.Println("\nRunning quick setup...")

	exe, err := os.Executable()
	if err != nil {
		log.Printf("Warning: could not find executable for install: %v", err)
	} else {
		installDir := installer.InstallDir()
		if err := installer.Install(installDir, exe); err != nil {
			log.Printf("Warning: install failed: %v", err)
		}
		installer.GenerateReadme(installDir)
	}

	cfg := config.DefaultConfig()
	hw, err := hardware.DetectHardware()
	if err == nil {
		cfg.Quantization = hardware.RecommendQuantization(hw)
		cfg.LlamaBackend = hardware.RecommendBackend(hw)
	}

	if detected := config.DetectExistingModels(); detected != nil {
		if detected.ModelPath != "" {
			cfg.ModelPathOverride = detected.ModelPath
			base := filepath.Base(detected.ModelPath)
			base = strings.TrimSuffix(base, ".gguf")
			repoParts := strings.Split(cfg.RepoID, "/")
			modelName := strings.TrimSuffix(repoParts[len(repoParts)-1], "-GGUF")
			quant := strings.TrimPrefix(base, modelName+"-")
			if quant != "" {
				cfg.Quantization = quant
			}
			fmt.Printf("Found existing model: %s\n", filepath.Base(detected.ModelPath))
		}
		if detected.MMProjPath != "" {
			cfg.MMProjPathOverride = detected.MMProjPath
			fmt.Printf("Found existing mmproj: %s\n", filepath.Base(detected.MMProjPath))
		}
		cfg.AutoDownload = false
		fmt.Println("Using existing models, auto-download disabled.")
	}

	if err := cfg.Save(); err != nil {
		log.Fatalf("Error saving config: %v", err)
	}

	fmt.Printf("Config saved to %s\n", config.ConfigPath())
	fmt.Printf("Backend: %s, Quantization: %s\n", cfg.LlamaBackend, cfg.Quantization)

	if cfg.AutoDownload {
		fmt.Println("Run 'vision-mcp' to start (models will download automatically).")
	} else {
		fmt.Println("Run 'vision-mcp' to start.")
	}

	promptMCPSetup()
}

// runWizardCmd launches the interactive 5-step TUI wizard for
// selecting model, backend, quantization, clipboard monitoring,
// then runs the download screen and prompts for MCP agent setup.
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

		exe, err := os.Executable()
		if err == nil {
			installDir := installer.InstallDir()
			if err := installer.Install(installDir, exe); err != nil {
				log.Printf("Warning: install failed: %v", err)
			} else {
				installer.GenerateReadme(installDir)
			}
		}

		promptMCPSetup()

		ds := setup.NewDownloadScreen(cfg)
		p := tea.NewProgram(ds)
		if _, err := p.Run(); err != nil {
			log.Printf("Warning: download screen error: %v", err)
		}

		fmt.Printf("\nSetup complete! Run 'vision-mcp' to start the server.\n")
	}
}

// runManualWizard launches the TUI for advanced users who already have
// model files (from LM Studio, Ollama, or custom paths) and want to
// point to them manually instead of auto-downloading.
func runManualWizard() {
	cfg, err := setup.RunManualWizard()
	if err != nil {
		log.Fatalf("Manual wizard error: %v", err)
	}
	if cfg != nil {
		if err := cfg.Save(); err != nil {
			log.Fatalf("Error saving config: %v", err)
		}
		fmt.Printf("\nConfiguration saved to %s\n", config.ConfigPath())

		exe, err := os.Executable()
		if err == nil {
			installDir := installer.InstallDir()
			if err := installer.Install(installDir, exe); err != nil {
				log.Printf("Warning: install failed: %v", err)
			} else {
				installer.GenerateReadme(installDir)
			}
		}

		fmt.Printf("Run 'vision-mcp' to start the server.\n")

		promptMCPSetup()
	}
}

// runMCPSetup detects installed AI coding agents (Kilo Code, OpenCode,
// PI Agent, Zed Editor) and interactively configures them to use
// vision-mcp as an MCP server.
func runMCPSetup() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("Error finding executable: %v", err)
	}

	agents := discover.DetectAgents(exe)

	if len(agents) == 0 {
		fmt.Println("No supported agents found (Kilo Code, OpenCode, PI Agent).")
		fmt.Println("Install an agent first, then run 'vision-mcp --mcp-setup'.")
		return
	}

	selected, err := setup.RunAgentSetup(agents)
	if err != nil {
		log.Fatalf("Agent setup error: %v", err)
	}

	if len(selected) == 0 {
		fmt.Println("MCP setup cancelled.")
		return
	}

	fmt.Println()

	for _, a := range selected {
		if a.Type == discover.AgentPi {
			piAgentDir := filepath.Join(homeDir(), ".pi", "agent", "settings.json")
			if data, err := os.ReadFile(piAgentDir); err == nil {
				if !strings.Contains(string(data), "pi-mcp-adapter") {
					fmt.Printf("%s requires pi-mcp-adapter.\n", a.Name)
					fmt.Print("Install pi-mcp-adapter now? [Y/n]: ")
					var input string
					fmt.Scanln(&input)
					input = strings.TrimSpace(strings.ToLower(input))
					if input == "" || input == "y" || input == "yes" {
						fmt.Println("Installing pi-mcp-adapter...")
						if err := discover.InstallPiMCPAdapter(); err != nil {
							log.Printf("Warning: install failed: %v", err)
							fmt.Println("Install manually: pi install npm:pi-mcp-adapter")
						}
					}
				}
			}
		}

		fmt.Printf("Configuring %s...\n", a.Name)
		if err := discover.ConfigureAgentMCP(a, exe); err != nil {
			log.Printf("Warning: failed to configure %s: %v", a.Name, err)
			fmt.Printf("  ✗ %s failed: %v\n", a.Name, err)
			continue
		}
		fmt.Printf("  ✓ %s configured!\n", a.Name)
	}

	if len(selected) > 0 {
		fmt.Println("\nMCP setup complete! Restart your agent to apply changes.")
	}
}

// homeDir returns the current user's home directory, or empty on error.
func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// promptMCPSetup is called after configuration completes. It detects
// agents and, if found, presents a TUI to configure them to use vision-mcp.
func promptMCPSetup() {
	exe, err := os.Executable()
	if err != nil {
		return
	}

	agents := discover.DetectAgents(exe)
	if len(agents) == 0 {
		return
	}

	selected, err := setup.RunAgentSetup(agents)
	if err != nil || len(selected) == 0 {
		return
	}

	fmt.Println()
	for _, a := range selected {
		if a.Type == discover.AgentPi {
			piSettings := filepath.Join(homeDir(), ".pi", "agent", "settings.json")
			if data, err := os.ReadFile(piSettings); err == nil {
				if !strings.Contains(string(data), "pi-mcp-adapter") {
					fmt.Printf("%s requires pi-mcp-adapter.\n", a.Name)
					fmt.Print("Install pi-mcp-adapter now? [Y/n]: ")
					var piInput string
					fmt.Scanln(&piInput)
					piInput = strings.TrimSpace(strings.ToLower(piInput))
					if piInput == "" || piInput == "y" || piInput == "yes" {
						fmt.Println("Installing pi-mcp-adapter...")
						if err := discover.InstallPiMCPAdapter(); err != nil {
							log.Printf("Warning: install failed: %v", err)
							fmt.Println("Install manually: pi install npm:pi-mcp-adapter")
						}
					}
				}
			}
		}

		fmt.Printf("Configuring %s...\n", a.Name)
		if err := discover.ConfigureAgentMCP(a, exe); err != nil {
			log.Printf("Warning: failed to configure %s: %v", a.Name, err)
			fmt.Printf("  ✗ %s failed: %v\n", a.Name, err)
			continue
		}
		fmt.Printf("  ✓ %s configured!\n", a.Name)
	}

	if len(selected) > 0 {
		fmt.Println("\nMCP setup complete! Restart your agent to apply changes.")
	}
}

// runInstallCmd copies the binary to ~/.go-mcp/vision/, creates
// shell launchers, adds the directory to PATH, and writes a config.
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

	promptMCPSetup()
}

// runUninstallCmd removes the install directory and prints a message
// about leftover PATH entries and config file.
func runUninstallCmd() {
	if err := installer.Uninstall(); err != nil {
		log.Fatalf("Uninstall error: %v", err)
	}
}

// runFreeMemory checks port 8001 for a running llama-server, sends
// SIGTERM, then kills the process. Falls back to killing any
// llama-server process by name (taskkill/pkill).
func runFreeMemory() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	if err := llamaserver.NewLock().ForceClear(); err == nil {
		log.Printf("Lock file cleared")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Port)

	resp, err := client.Get(healthURL)
	if err != nil {
		fmt.Printf("No llama-server responding on port %d\n", cfg.Port)
		killAnyLlamaServer()
		return
	}
	resp.Body.Close()

	pid := findProcessOnPort(cfg.Port)
	if pid > 0 {
		proc, err := os.FindProcess(pid)
		if err == nil {
			proc.Signal(syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			proc.Kill()
			fmt.Println("Model unloaded, memory freed.")
			return
		}
	}
	fmt.Println("Could not find process to free.")
	killAnyLlamaServer()
}

// killAnyLlamaServer uses taskkill (Windows) or pkill (Unix) to
// forcefully terminate all llama-server processes.
func killAnyLlamaServer() {
	bin := "llama-server"
	if runtime.GOOS == "windows" {
		bin = "llama-server.exe"
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("taskkill", "/F", "/IM", bin)
	} else {
		cmd = exec.Command("pkill", "-f", "llama-server")
	}
	if err := cmd.Run(); err != nil {
		fmt.Println("No llama-server processes found.")
	} else {
		fmt.Println("Killed remaining llama-server processes.")
	}
}

// findProcessOnPort returns the PID of the process listening on the
// given TCP port. Uses netstat (Windows) or lsof (Unix).
func findProcessOnPort(port int) int {
	pid := 0
	portStr := fmt.Sprintf(":%d ", port)

	if runtime.GOOS == "windows" {
		out, err := exec.Command("netstat", "-ano").Output()
		if err != nil {
			return 0
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "LISTENING") && strings.Contains(line, portStr) {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					fmt.Sscanf(fields[len(fields)-1], "%d", &pid)
				}
			}
		}
	} else {
		out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
		if err != nil {
			return 0
		}
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid)
	}
	return pid
}

// resolveLlamaServer returns the path to a llama-server binary.
// Three modes:
//   - "auto": downloads the binary matching LlamaBackend
//   - "custom": uses the user-configured LlamaServerPath
//   - "" (default): looks in PATH first, falls back to auto-download
// Returns (resolvedPath, newPathIfDownloaded, error).
func resolveLlamaServer(cfg *config.Config) (binPath, newPath string, err error) {
	mode := cfg.LlamaServerMode
	if mode == "" && cfg.LlamaServerPath == "auto-download" {
		mode = "auto"
	}

	switch mode {
	case "auto":
		log.Printf("Downloading llama-server...")
		binPath, err = download.EnsureLlamaBinary(cfg.LlamaBackend, cfg.LlamaServerDir, downloadProgress("llama-server"))
		if err != nil {
			return "", "", err
		}
		log.Printf("llama-server downloaded to: %s", binPath)
		return binPath, binPath, nil

	case "custom":
		if cfg.LlamaServerPath == "" {
			return "", "", fmt.Errorf("llama_server_mode is 'custom' but llama_server_path is empty")
		}
		binPath = cfg.LlamaServerPath
		if fi, err := os.Stat(binPath); err == nil && fi.IsDir() {
			name := "llama-server"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			binPath = filepath.Join(binPath, name)
		}
		log.Printf("Using configured llama-server: %s", binPath)
		return binPath, "", nil

	default:
		found, lookupErr := discover.FindSystemLlamaServer()
		if lookupErr == nil {
			log.Printf("Using llama-server from PATH: %s", found)
			return found, "", nil
		}
		log.Printf("llama-server not found, downloading...")
		binPath, downloadErr := download.EnsureLlamaBinary(cfg.LlamaBackend, cfg.LlamaServerDir, downloadProgress("llama-server"))
		if downloadErr != nil {
			return "", "", downloadErr
		}
		log.Printf("llama-server downloaded to: %s", binPath)
		cfg.LlamaServerMode = "auto"
		return binPath, binPath, nil
	}
}

// runServer starts the MCP server in STDIO mode. It performs async
// initialization: hardware detection, port reuse check, model download
// (lazy), and llama-server startup (lazy on first tool call).
// Also starts idle timeout monitor and signal handler goroutines.
func runServer() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}
	cfg.Save()

	mcpServer := server.NewMCPServer("vision-mcp", "1.0.0")
	handler := mcptools.NewToolHandler("", cfg.CustomPrompt)
	handler.RegisterTools(mcpServer)

	var clipMon *clipboard.Monitor
	if cfg.ClipboardMonitorEnabled {
		clipMon = clipboard.NewMonitor(cfg)
		clipMon.Start()
		handler.SetClipboardMonitor(clipMon)
		log.Printf("Clipboard monitor enabled (history limit: %d)", cfg.ClipboardHistoryLimit)
	}

	go func() {
		hw, err := hardware.DetectHardware()
		if err == nil {
			log.Printf("Hardware: RAM=%dGB VRAM=%dGB",
				hw.TotalRAM/(1024*1024*1024),
				hw.GPU.VRAM/(1024*1024*1024))

			if cfg.LlamaBackend == "" || cfg.LlamaBackend == "cuda" && !hw.GPU.Present {
				cfg.LlamaBackend = hardware.RecommendBackend(hw)
			}
			if cfg.LlamaBackend == "cpu" {
				cfg.NGL = 0
			}
			cfg.Save()
		}

		lck := llamaserver.NewLock()

		healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Port)
		hc := &http.Client{Timeout: 2 * time.Second}
		alreadyRunning := false
		if resp, err := hc.Get(healthURL); err == nil {
			resp.Body.Close()
			log.Printf("llama-server already running on port %d, reusing", cfg.Port)
			handler.SetLlamaURL(fmt.Sprintf("http://127.0.0.1:%d", cfg.Port))
			handler.SetLoaded(true)
			alreadyRunning = true
			if err := lck.AddPID(); err != nil {
				log.Printf("Warning: failed to register in lock: %v", err)
			}
		}

		_, cancel := context.WithCancel(context.Background())
		var srv *llamaserver.Server

		handler.SetStopFunc(func() {
			cancel()
			handler.SetLoaded(false)
			if clipMon != nil {
				clipMon.Stop()
			}

			shouldStop, _ := lck.Release()
			if shouldStop && srv != nil {
				srv.Stop()
			} else {
				log.Printf("llama-server kept alive for other MCP processes")
			}
		})

		assetsReady := make(chan struct{})

		if !alreadyRunning && cfg.AutoDownload {
			go func() {
				downloadErr := func() error {
					if err := download.EnsureModels(cfg, downloadProgress("Model")); err != nil {
						return err
					}
					_, newLlamaPath, err := resolveLlamaServer(cfg)
					if err != nil {
						return err
					}
					if newLlamaPath != "" {
						cfg.LlamaServerPath = newLlamaPath
					}
					cfg.ModelPathOverride = cfg.ModelPath()
					cfg.MMProjPathOverride = cfg.MMProjPath()
					cfg.Save()
					return nil
				}()
				if downloadErr != nil {
					log.Printf("Warning: background asset download failed: %v", downloadErr)
				}
				close(assetsReady)
			}()
		} else {
			close(assetsReady)
		}

		handler.SetRestartFunc(func(restartCtx context.Context) error {
			select {
			case <-assetsReady:
			case <-restartCtx.Done():
				return restartCtx.Err()
			}

			if cfg.AutoDownload {
				if err := download.EnsureModels(cfg, downloadProgress("Model")); err != nil {
					return fmt.Errorf("download models: %w", err)
				}
			}

			llamaBin, newLlamaPath, err := resolveLlamaServer(cfg)
			if err != nil {
				return fmt.Errorf("resolve llama-server: %w", err)
			}
			if newLlamaPath != "" {
				cfg.LlamaServerPath = newLlamaPath
				cfg.Save()
			}

			newSrv := llamaserver.New(
				cfg.ModelPath(),
				cfg.MMProjPath(),
				cfg.Port,
				cfg.NGL,
				cfg.NCtx,
				cfg.FlashAttn,
				llamaBin,
				cfg.KvCacheTypeK,
				cfg.KvCacheTypeV,
			)
			if err := newSrv.Start(restartCtx); err != nil {
				return fmt.Errorf("start llama-server: %w", err)
			}
			srv = newSrv
			lck.Start(cfg.Port)
			handler.SetLlamaURL(srv.URL())
			handler.SetLoaded(true)
			return nil
		})

		handler.SetReady()

		if cfg.IdleTimeout > 0 {
			idleDuration := time.Duration(cfg.IdleTimeout) * time.Minute
			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					if !handler.IsLoaded() {
						continue
					}
					if handler.IdleTime() > idleDuration {
						log.Printf("Idle timeout (%d min), freeing GPU memory", cfg.IdleTimeout)
						cancel()
						handler.SetLoaded(false)
						lck.ForceClear()
						if srv != nil {
							srv.Stop()
						} else {
							pid := findProcessOnPort(cfg.Port)
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
			}()
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			log.Printf("Shutting down...")
			handler.Stop()
			os.Exit(0)
		}()
	}()

	log.Printf("MCP server ready (STDIO mode)")
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Printf("MCP server error: %v", err)
	}

	log.Printf("MCP client disconnected, cleaning up...")
	handler.Stop()
}

// runAnalyzeClipboard is the --analyze-clipboard CLI entrypoint.
// It reads the clipboard image, ensures models and llama-server are
// ready (downloading if needed), starts llama-server, and prints the
// model's answer to stdout.
func runAnalyzeClipboard(prompt string) {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	cfg.Save()

	fmt.Fprintf(os.Stderr, "Checking clipboard...\n")
	if _, err := mcptools.ClipboardImageDataURI(); err != nil {
		fmt.Fprintf(os.Stderr, "Clipboard error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Checking models...\n")
	if err := download.EnsureModels(cfg, downloadProgress("Model")); err != nil {
		fmt.Fprintf(os.Stderr, "Download models: %v\n", err)
		os.Exit(1)
	}

	llamaBin, _, err := resolveLlamaServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve llama-server: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Starting llama-server...\n")
	srv := llamaserver.New(
		cfg.ModelPath(),
		cfg.MMProjPath(),
		cfg.Port,
		cfg.NGL,
		cfg.NCtx,
		cfg.FlashAttn,
		llamaBin,
		cfg.KvCacheTypeK,
		cfg.KvCacheTypeV,
	)
	if err := srv.Start(ctx); err != nil {
		srv.Stop()
		fmt.Fprintf(os.Stderr, "Start llama-server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Stop()

	fmt.Fprintf(os.Stderr, "Reading clipboard and analyzing...\n")
	result, err := mcptools.CLIAnalyzeClipboard(ctx, prompt, srv.URL(), &http.Client{})
	if err != nil {
		srv.Stop()
		fmt.Fprintf(os.Stderr, "Analysis failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

// displayStatus prints the current config, hardware profile,
// recommended backend and quantization, and available tools.
func displayStatus() {
	cfg, _ := config.LoadConfig()
	fmt.Println("Vision MCP Status")
	fmt.Println("=================")
	fmt.Printf("Config:       %s\n", config.ConfigPath())
	fmt.Printf("Quantization: %s\n", cfg.Quantization)
	fmt.Printf("MMProj:       %s\n", cfg.MMProj)
	fmt.Printf("Backend:      %s\n", cfg.LlamaBackend)
	fmt.Printf("Port:         %d\n", cfg.Port)
	fmt.Printf("Idle timeout: %d min\n", cfg.IdleTimeout)
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
	fmt.Println("  analyze_clipboard(prompt)")
}

// runDownload is the --download entrypoint. It ensures model files
// (GGUF + mmproj) exist, downloading from HuggingFace if needed.
func runDownload() {
	cfg, _ := config.LoadConfig()
	fmt.Println("Downloading models...")
	if err := download.EnsureModels(cfg, downloadProgress("Model")); err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Println("Models ready.")
}

// downloadProgress returns a ProgressFunc callback that logs download
// progress (percentage and bytes) to the application log.
func downloadProgress(label string) download.ProgressFunc {
	return func(downloaded, total int64) {
		if total > 0 && downloaded > 0 {
			pct := float64(downloaded) / float64(total) * 100
			log.Printf("%s: %.1f%% (%s/%s)",
				label, pct,
				download.FormatBytes(downloaded),
				download.FormatBytes(total))
		}
		if downloaded == total && total > 0 {
			log.Printf("%s: 100%% (%s) ✓",
				label, download.FormatBytes(total))
		}
	}
}
