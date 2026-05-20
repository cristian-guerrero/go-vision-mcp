//go:build windows

package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/image/bmp"
	"golang.org/x/sys/windows"
)

// Win32 clipboard format constants.
const (
	cfBitmap = 2
	cfDIB    = 8
	cfHDROP  = 15
	cfDIBV5  = 17
)

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")

	procOpenClipboard              = modUser32.NewProc("OpenClipboard")
	procCloseClipboard             = modUser32.NewProc("CloseClipboard")
	procIsClipboardFormatAvailable = modUser32.NewProc("IsClipboardFormatAvailable")
	procGetClipboardData           = modUser32.NewProc("GetClipboardData")
	procGlobalLock                 = modKernel32.NewProc("GlobalLock")
	procGlobalUnlock               = modKernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = modKernel32.NewProc("GlobalSize")
	procDragQueryFileW             = modShell32.NewProc("DragQueryFileW")
)

// ReadClipboardImage reads an image from the Windows clipboard using
// direct Win32 API calls. It checks CF_HDROP (file drops) first, then
// CF_DIBV5, then CF_DIB.
func ReadClipboardImage() (rawPNG []byte, originalPath string, fileType string, err error) {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return nil, "", "", fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()

	// Try file drop list first (CF_HDROP)
	hDrop, _, _ := procGetClipboardData.Call(uintptr(cfHDROP))
	if hDrop != 0 {
		path, ftype := readHDROP(hDrop)
		if path != "" {
			return nil, path, ftype, nil
		}
	}

	// Try DIBV5 (modern bitmaps with alpha)
	if isFormatAvailable(cfDIBV5) {
		data, err := readDIB(cfDIBV5)
		if err == nil {
			pngData, err := dibToPNG(data)
			if err == nil {
				return pngData, "", "png", nil
			}
		}
	}

	// Try DIB (standard bitmap)
	if isFormatAvailable(cfDIB) {
		data, err := readDIB(cfDIB)
		if err == nil {
			pngData, err := dibToPNG(data)
			if err == nil {
				return pngData, "", "png", nil
			}
		}
	}

	return nil, "", "", fmt.Errorf("no image found in clipboard")
}

// isFormatAvailable checks whether a clipboard format is currently
// available using IsClipboardFormatAvailable.
func isFormatAvailable(format uint32) bool {
	ret, _, _ := procIsClipboardFormatAvailable.Call(uintptr(format))
	return ret != 0
}

// readDIB retrieves raw DIB (Device Independent Bitmap) data for the
// given clipboard format using GetClipboardData and GlobalLock.
func readDIB(format uint32) ([]byte, error) {
	hMem, _, _ := procGetClipboardData.Call(uintptr(format))
	if hMem == 0 {
		return nil, fmt.Errorf("GetClipboardData returned 0")
	}

	size, _, _ := procGlobalSize.Call(hMem)
	if size == 0 {
		return nil, fmt.Errorf("GlobalSize returned 0")
	}

	var lockedPtr uintptr
	lockedPtr, _, _ = procGlobalLock.Call(hMem)
	if lockedPtr == 0 {
		return nil, fmt.Errorf("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(hMem)

	// GlobalLock returns a pointer to pinned Windows heap memory (not
	// Go GC memory), so the uintptr→unsafe.Pointer conversion is safe.
	// We construct a slice header manually to avoid a Go vet false positive.
	hdr := struct {
		addr uintptr
		len  int
		cap  int
	}{lockedPtr, int(size), int(size)}
	data := *(*[]byte)(unsafe.Pointer(&hdr))
	return data, nil
}

// readHDROP parses a CF_HDROP handle to extract the first file path
// and its image type. Returns empty strings if no supported image is
// found in the file drop list.
func readHDROP(hDrop uintptr) (path string, fileType string) {
	count, _, _ := procDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)
	if count == 0 {
		return "", ""
	}

	buf := make([]uint16, 32768)
	n, _, _ := procDragQueryFileW.Call(hDrop, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return "", ""
	}

	path = windows.UTF16ToString(buf[:n])
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp":
		return path, "file"
	case ".webp":
		return path, "webp"
	case ".avif":
		return path, "avif"
	default:
		return "", ""
	}
}

// dibToPNG converts DIB bitmap bytes to PNG by wrapping them in a BMP
// structure and decoding/re-encoding via the standard library.
func dibToPNG(dib []byte) ([]byte, error) {
	if len(dib) < 40 {
		return nil, fmt.Errorf("dib: data too short (%d bytes)", len(dib))
	}

	bihSize := int(binary.LittleEndian.Uint32(dib[0:4]))
	if bihSize < 40 || bihSize > len(dib) {
		return nil, fmt.Errorf("dib: invalid header size %d", bihSize)
	}

	bitCount := binary.LittleEndian.Uint16(dib[14:16])
	compression := binary.LittleEndian.Uint32(dib[16:20])

	// Color table for paletted bitmaps (<= 8 bpp)
	var colorTableSize int
	if bitCount <= 8 {
		clrUsed := int(binary.LittleEndian.Uint32(dib[32:36]))
		if clrUsed > 0 {
			colorTableSize = clrUsed * 4
		} else {
			colorTableSize = (1 << bitCount) * 4
		}
	}

	// Extra RGB bit masks for BI_BITFIELDS (compression == 3)
	var extraMasks int
	if compression == 3 {
		extraMasks = 12
	}

	offBits := 14 + bihSize + extraMasks + colorTableSize

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0x4D42)) // 'BM'
	binary.Write(&buf, binary.LittleEndian, uint32(14+len(dib)))
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint32(offBits))
	buf.Write(dib)

	img, err := bmp.Decode(&buf)
	if err != nil {
		return nil, fmt.Errorf("decode dib: %w", err)
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return pngBuf.Bytes(), nil
}
