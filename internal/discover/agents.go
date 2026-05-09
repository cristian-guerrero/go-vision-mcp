package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/vision-mcp/internal/config"
)

type AgentType string

const (
	AgentKilo     AgentType = "kilo"
	AgentOpenCode AgentType = "opencode"
	AgentPi       AgentType = "pi"
	AgentZed      AgentType = "zed"
)

type AgentInfo struct {
	Type      AgentType
	Name      string
	ConfigPath string
	Installed bool
	Configured bool
}

type MCPConfig struct {
	Type    string   `json:"type"`
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Enabled *bool    `json:"enabled,omitempty"`
}

type KiloConfig struct {
	MCP map[string]MCPConfig `json:"mcp"`
}

type OpenCodeConfig struct {
	MCP map[string]MCPConfig `json:"mcp"`
}

type PiMCPEntry struct {
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

type PiMCPConfig struct {
	MCPServers map[string]PiMCPEntry `json:"mcpServers"`
}

type ZedContextServer struct {
	Command []string          `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type ZedSettings struct {
	ContextServers map[string]ZedContextServer `json:"context_servers"`
}

func DetectAgents(binaryPath string) []AgentInfo {
	agents := []AgentInfo{
		detectKilo(binaryPath),
		detectOpenCode(binaryPath),
		detectPi(binaryPath),
		detectZed(binaryPath),
	}

	var result []AgentInfo
	for _, a := range agents {
		if a.Installed {
			result = append(result, a)
		}
	}
	return result
}

func detectKilo(binaryPath string) AgentInfo {
	info := AgentInfo{Type: AgentKilo, Name: "Kilo Code"}

	configPath := filepath.Join(homeDir(), ".config", "kilo", "kilo.json")
	info.ConfigPath = configPath

	if _, err := os.Stat(configPath); err != nil {
		return info
	}
	info.Installed = true

	data, err := os.ReadFile(configPath)
	if err != nil {
		return info
	}

	var cfg KiloConfig
	if json.Unmarshal(data, &cfg) == nil {
		if _, exists := cfg.MCP["vision-mcp"]; exists {
			info.Configured = true
		}
	}

	return info
}

func detectOpenCode(binaryPath string) AgentInfo {
	info := AgentInfo{Type: AgentOpenCode, Name: "OpenCode"}

	configPath := filepath.Join(homeDir(), ".config", "opencode", "opencode.json")
	info.ConfigPath = configPath

	if _, err := os.Stat(configPath); err != nil {
		return info
	}
	info.Installed = true

	data, err := os.ReadFile(configPath)
	if err != nil {
		return info
	}

	var cfg OpenCodeConfig
	if json.Unmarshal(data, &cfg) == nil {
		if _, exists := cfg.MCP["vision-mcp"]; exists {
			info.Configured = true
		}
	}

	return info
}

func detectPi(binaryPath string) AgentInfo {
	info := AgentInfo{Type: AgentPi, Name: "PI Agent"}

	settingsPath := filepath.Join(homeDir(), ".pi", "agent", "settings.json")
	info.ConfigPath = settingsPath

	if _, err := os.Stat(settingsPath); err != nil {
		return info
	}
	info.Installed = true

	mcpConfigPath := filepath.Join(homeDir(), ".config", "mcp", "mcp.json")
	info.ConfigPath = mcpConfigPath

	if data, err := os.ReadFile(mcpConfigPath); err == nil {
		var cfg PiMCPConfig
		if json.Unmarshal(data, &cfg) == nil {
			if _, exists := cfg.MCPServers["vision-mcp"]; exists {
				info.Configured = true
			}
		}
	}

	return info
}

func zedConfigPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "Zed", "settings.json")
	}
	return filepath.Join(homeDir(), ".config", "zed", "settings.json")
}

func detectZed(binaryPath string) AgentInfo {
	info := AgentInfo{Type: AgentZed, Name: "Zed Editor"}

	configPath := zedConfigPath()
	info.ConfigPath = configPath

	if _, err := os.Stat(configPath); err != nil {
		return info
	}
	info.Installed = true

	data, err := os.ReadFile(configPath)
	if err != nil {
		return info
	}

	var cfg ZedSettings
	if json.Unmarshal(data, &cfg) == nil {
		if _, exists := cfg.ContextServers["vision-mcp"]; exists {
			info.Configured = true
		}
	}

	return info
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

func InstallPiMCPAdapter() error {
	cmd := exec.Command("pi", "install", "npm:pi-mcp-adapter")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ConfigureAgentMCP(agent AgentInfo, binaryPath string) error {
	cmdName := resolveCommandPath(binaryPath)

	switch agent.Type {
	case AgentKilo:
		return configureKiloMCP(agent, cmdName)
	case AgentOpenCode:
		return configureOpenCodeMCP(agent, cmdName)
	case AgentPi:
		return configurePiMCP(agent, cmdName)
	case AgentZed:
		return configureZedMCP(agent, cmdName)
	}
	return fmt.Errorf("unknown agent type: %s", agent.Type)
}

func resolveCommandPath(binaryPath string) string {
	installDir := config.InstallDir()
	installedExe := filepath.Join(installDir, executableName())

	if _, err := os.Stat(installedExe); err == nil {
		if runtime.GOOS == "windows" {
			launcher := filepath.Join(installDir, "vision-mcp.cmd")
			if _, err := os.Stat(launcher); err == nil {
				return launcher
			}
		}
		return installedExe
	}

	if runtime.GOOS == "windows" {
		return binaryPath
	}
	return "vision-mcp"
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "vision-mcp.exe"
	}
	return "vision-mcp"
}

func configureKiloMCP(agent AgentInfo, binaryPath string) error {
	path := filepath.Join(homeDir(), ".config", "kilo", "kilo.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	mcp, _ := raw["mcp"].(map[string]any)
	if mcp == nil {
		mcp = make(map[string]any)
		raw["mcp"] = mcp
	}

	existing, _ := mcp["vision-mcp"].(map[string]any)
	entry := map[string]any{
		"type":    "local",
		"command": []string{binaryPath},
		"enabled": true,
	}
	if existing != nil {
		for k, v := range existing {
			if _, core := coreFields[k]; !core {
				entry[k] = v
			}
		}
	}
	mcp["vision-mcp"] = entry

	return writeJSON(path, raw)
}

var coreFields = map[string]bool{
	"type": true, "command": true, "enabled": true,
}

func configureOpenCodeMCP(agent AgentInfo, binaryPath string) error {
	path := filepath.Join(homeDir(), ".config", "opencode", "opencode.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	mcp, _ := raw["mcp"].(map[string]any)
	if mcp == nil {
		mcp = make(map[string]any)
		raw["mcp"] = mcp
	}

	existing, _ := mcp["vision-mcp"].(map[string]any)
	entry := map[string]any{
		"type":    "local",
		"command": []string{binaryPath},
		"enabled": true,
	}
	if existing != nil {
		for k, v := range existing {
			if _, core := coreFields[k]; !core {
				entry[k] = v
			}
		}
	}
	mcp["vision-mcp"] = entry

	return writeJSON(path, raw)
}

func configurePiMCP(agent AgentInfo, binaryPath string) error {
	os.MkdirAll(filepath.Join(homeDir(), ".config", "mcp"), 0755)

	mcpPath := filepath.Join(homeDir(), ".config", "mcp", "mcp.json")

	var cfg PiMCPConfig
	if data, err := os.ReadFile(mcpPath); err == nil {
		json.Unmarshal(data, &cfg)
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]PiMCPEntry)
	}

	if _, exists := cfg.MCPServers["vision-mcp"]; !exists {
		cfg.MCPServers["vision-mcp"] = PiMCPEntry{
			Command: binaryPath,
		}
	}

	return writeJSON(mcpPath, cfg)
}

func configureZedMCP(agent AgentInfo, binaryPath string) error {
	path := zedConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	servers, _ := raw["context_servers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
		raw["context_servers"] = servers
	}

	existing, _ := servers["vision-mcp"].(map[string]any)
	entry := map[string]any{
		"command": []string{binaryPath},
		"args":    []string{},
		"env":     map[string]any{},
	}
	if existing != nil {
		for k, v := range existing {
			entry[k] = v
		}
		entry["command"] = []string{binaryPath}
		if _, ok := entry["args"]; !ok {
			entry["args"] = []string{}
		}
		if _, ok := entry["env"]; !ok {
			entry["env"] = map[string]any{}
		}
	}
	servers["vision-mcp"] = entry

	return writeJSON(path, raw)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
