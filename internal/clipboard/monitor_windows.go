//go:build windows

package clipboard

import (
	"golang.org/x/sys/windows"
)

// FOLDERID_Screenshots — resolves to the user's actual
// Screenshots folder regardless of OneDrive redirection or
// localization.
var folderIDScreenshots = &windows.KNOWNFOLDERID{
	Data1: 0xb7bede81,
	Data2: 0xdf94,
	Data3: 0x4682,
	Data4: [8]byte{0xa7, 0xd8, 0x57, 0xa5, 0x26, 0x20, 0xb8, 0x6f},
}

// knownFolderPath is a test-friendly variable so we can swap the
// implementation in unit tests without calling the real Win32 API.
var knownFolderPath = windows.KnownFolderPath

// defaultScreenshotFolder uses the Win32 Known Folder API to find
// the user's actual Screenshots folder. On Windows 10+ this is
// typically %USERPROFILE%\Pictures\Screenshots, but may be
// redirected to OneDrive or a custom location.
func defaultScreenshotFolder() string {
	path, err := knownFolderPath(folderIDScreenshots, 0)
	if err != nil {
		return ""
	}
	return path
}

// clipboardPollImage reads the clipboard using the Win32 API directly.
// Returns nil when no image is in the clipboard (no error).
func clipboardPollImage() (*PollResult, error) {
	pngData, origPath, _, err := ReadClipboardImage()
	if err != nil {
		return nil, nil
	}
	if origPath != "" {
		return &PollResult{OriginalPath: origPath}, nil
	}
	return &PollResult{Data: pngData}, nil
}
