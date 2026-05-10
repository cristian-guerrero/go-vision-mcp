package image

import (
	"bytes"
	"fmt"
	"image/png"

	"golang.org/x/image/webp"
)

func DecodeWebPToPNG(raw []byte) ([]byte, error) {
	img, err := webp.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode webp: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode webp to png: %w", err)
	}
	return buf.Bytes(), nil
}
