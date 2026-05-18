package image

import (
	"bytes"
	"fmt"
	stdimage "image"
	"image/jpeg"

	_ "github.com/gen2brain/avif"
)

// DecodeAVIFToJPEG decodes AVIF image bytes and re-encodes them as JPEG.
// This is needed because llama-server does not accept AVIF images.
func DecodeAVIFToJPEG(raw []byte) ([]byte, error) {
	img, _, err := stdimage.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode avif: %w", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode avif to jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
