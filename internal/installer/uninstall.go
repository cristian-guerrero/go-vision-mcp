package installer

import (
	"fmt"
	"os"

	"github.com/cristian-guerrero/go-vision-mcp/internal/config"
)

func Uninstall() error {
	installDir := InstallDir()

	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		fmt.Println("No installation found.")
		return nil
	}

	fmt.Printf("Removing %s...\n", installDir)
	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("remove install dir: %w", err)
	}

	fmt.Println("Installation removed.")

	fmt.Println()
	fmt.Println("Note: PATH entries were not automatically removed.")
	fmt.Println("If vision-mcp was added to your PATH, you may need to remove it manually:")
	fmt.Printf("  - Windows: Open System Properties > Environment Variables\n")
	fmt.Printf("  - Linux/macOS: Edit ~/.bashrc or ~/.zshrc and remove the line with '%s'\n", installDir)

	cfgPath := config.ConfigPath()
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("Config file remains at %s\n", cfgPath)
	}

	return nil
}
