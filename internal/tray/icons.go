// Package tray builds the status icons shown in the system tray.
package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"sync"
)

// Colors per state name (matches session.State values).
var colors = map[string]color.RGBA{
	"idle":      {0x4C, 0x8D, 0xF6, 0xFF}, // blue: waiting for the wake word
	"listening": {0x2E, 0xCC, 0x71, 0xFF}, // green
	"thinking":  {0xF5, 0xA6, 0x23, 0xFF}, // amber
	"speaking":  {0x9B, 0x59, 0xD6, 0xFF}, // purple
	"paused":    {0x95, 0xA5, 0xA6, 0xFF}, // gray
}

// Color returns the color used for a state (idle color for unknown states).
func Color(state string) color.RGBA {
	if c, ok := colors[state]; ok {
		return c
	}
	return colors["idle"]
}

const size = 32

// PNG draws an anti-aliased filled circle in the state's color.
func PNG(state string) []byte {
	c := Color(state)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	cx, cy, r := float64(size)/2, float64(size)/2, float64(size)/2-2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			a := math.Max(0, math.Min(1, r-d+0.5))
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(a * 255)})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// ICO wraps a PNG in an .ico container (Windows requires .ico for tray icons).
func ICO(pngData []byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0, 1, 0, 1, 0})
	b.Write([]byte{size, size, 0, 0, 1, 0, 32, 0})
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pngData)))
	_ = binary.Write(&b, binary.LittleEndian, uint32(22))
	b.Write(pngData)
	return b.Bytes()
}

var (
	mu    sync.Mutex
	cache = map[string][]byte{}
)

// Icon returns the icon bytes in the format the current OS expects.
func Icon(state string) []byte {
	mu.Lock()
	defer mu.Unlock()
	key := state + "/" + runtime.GOOS
	if b, ok := cache[key]; ok {
		return b
	}
	b := PNG(state)
	if runtime.GOOS == "windows" {
		b = ICO(b)
	}
	cache[key] = b
	return b
}
