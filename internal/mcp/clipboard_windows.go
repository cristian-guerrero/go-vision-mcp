//go:build windows

package mcp

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func clipboardImageDataURIImpl() (string, error) {
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, "vision-mcp-clipboard.png")

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$img = $null

$img = [System.Windows.Forms.Clipboard]::GetImage()

if ($img -eq $null) {
	$files = [System.Windows.Forms.Clipboard]::GetFileDropList()
	if ($files -ne $null -and $files.Count -gt 0) {
		$ext = [System.IO.Path]::GetExtension($files[0]).ToLower()
		if ($ext -in '.png','.jpg','.jpeg','.gif','.bmp','.webp') {
			$img = [System.Drawing.Image]::FromFile($files[0])
		}
	}
}

if ($img -eq $null) {
	$data = [System.Windows.Forms.Clipboard]::GetData("Bitmap")
	if ($data -ne $null) {
		$img = $data
	}
}

if ($img -eq $null) { Write-Output "no_image"; exit 1 }

$bmp = New-Object System.Drawing.Bitmap($img)
$bmp.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
$img.Dispose()
exit 0
`, strings.ReplaceAll(tmpPath, "'", "''"))

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "no_image") {
			return "", fmt.Errorf("no image found in clipboard")
		}
		return "", fmt.Errorf("clipboard read failed: %v - %s", err, msg)
	}

	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("read clipboard image: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:image/png;base64,%s", b64), nil
}
