//go:build windows

package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// clipboardPollImage uses PowerShell to check for a new clipboard image.
// It tries GetImage → GetFileDropList (image files, WebP, AVIF) → GetData("Bitmap").
// Returns nil when no image is in the clipboard (no error).
func clipboardPollImage() (*PollResult, error) {
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, "vision-mcp-clipboard-monitor.png")

	escPath := strings.ReplaceAll(tmpPath, "'", "''")

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$img = [System.Windows.Forms.Clipboard]::GetImage()

if ($img -eq $null) {
	$files = [System.Windows.Forms.Clipboard]::GetFileDropList()
	if ($files -ne $null -and $files.Count -gt 0) {
		$origPath = $files[0]
		$ext = [System.IO.Path]::GetExtension($origPath).ToLower()
		if ($ext -eq '.webp') {
			Write-Output ("file_webp:" + $origPath)
			exit 0
		} elseif ($ext -eq '.avif') {
			Write-Output ("file_avif:" + $origPath)
			exit 0
		} elseif ($ext -in '.png','.jpg','.jpeg','.gif','.bmp') {
			Write-Output ("file:" + $origPath)
			exit 0
		}
	}
}

if ($img -eq $null) {
	$data = [System.Windows.Forms.Clipboard]::GetData("Bitmap")
	if ($data -ne $null) { $img = $data }
}

if ($img -eq $null) { Write-Output "no_image"; exit 1 }

$bmp = New-Object System.Drawing.Bitmap($img)
$bmp.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
$img.Dispose()
Write-Output "raw"
exit 0
`, escPath)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "no_image") {
			return nil, nil
		}
		return nil, fmt.Errorf("clipboard monitor read: %v - %s", err, msg)
	}

	outStr := strings.TrimSpace(string(out))
	lines := strings.Split(outStr, "\n")
	lastLine := strings.TrimSpace(lines[len(lines)-1])

	if strings.HasPrefix(lastLine, "file:") {
		origPath := strings.TrimPrefix(lastLine, "file:")
		if _, err := os.Stat(origPath); err == nil {
			return &PollResult{OriginalPath: origPath}, nil
		}
	}

	if strings.HasPrefix(lastLine, "file_webp:") {
		origPath := strings.TrimPrefix(lastLine, "file_webp:")
		if _, err := os.Stat(origPath); err == nil {
			return &PollResult{OriginalPath: origPath}, nil
		}
	}

	if strings.HasPrefix(lastLine, "file_avif:") {
		origPath := strings.TrimPrefix(lastLine, "file_avif:")
		if _, err := os.Stat(origPath); err == nil {
			return &PollResult{OriginalPath: origPath}, nil
		}
	}

	if lastLine == "raw" {
		defer os.Remove(tmpPath)
		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return nil, fmt.Errorf("read clipboard image: %w", err)
		}
		return &PollResult{Data: data}, nil
	}

	return nil, nil
}
