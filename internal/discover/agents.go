package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type AgentType string

const (
	AgentKilo    AgentType = "kilo"
	AgentOpenCode AgentType = "opencode"
	AgentPi      AgentType = "pi"
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

func DetectAgents(binaryPath string) []AgentInfo {
	agents := []AgentInfo{
		detectKilo(binaryPath),
		detectOpenCode(binaryPath),
		detectPi(binaryPath),
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
	cmdName := "vision-mcp"
	if runtime.GOOS == "windows" {
		cmdName = binaryPath
	}

	switch agent.Type {
	case AgentKilo:
		return configureKiloMCP(agent, cmdName)
	case AgentOpenCode:
		return configureOpenCodeMCP(agent, cmdName)
	case AgentPi:
		return configurePiMCP(agent, cmdName)
	}
	return fmt.Errorf("unknown agent type: %s", agent.Type)
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

	if _, exists := mcp["vision-mcp"]; !exists {
		mcp["vision-mcp"] = map[string]any{
			"type":    "local",
			"command": []string{binaryPath},
		}
	}

	return writeJSON(path, raw)
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

	if _, exists := mcp["vision-mcp"]; !exists {
		enabled := true
		mcp["vision-mcp"] = map[string]any{
			"type":    "local",
			"command": []string{binaryPath},
			"enabled": &enabled,
		}
	}

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

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
