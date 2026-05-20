//go:build windows

package clipboard

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
