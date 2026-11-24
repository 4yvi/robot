package tray

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestPNGIsValidAndColored(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(PNG("listening")))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(16, 16).RGBA()
	want := Color("listening")
	if uint8(r>>8) != want.R || uint8(g>>8) != want.G || uint8(b>>8) != want.B || a == 0 {
		t.Fatalf("center pixel %v %v %v %v", r>>8, g>>8, b>>8, a>>8)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Fatal("corner must be transparent")
	}
}

func TestStatesHaveDistinctColors(t *testing.T) {
	seen := map[[3]uint8]string{}
	for _, s := range []string{"idle", "listening", "thinking", "speaking", "paused"} {
		c := Color(s)
		k := [3]uint8{c.R, c.G, c.B}
		if prev, dup := seen[k]; dup {
			t.Fatalf("%s and %s share a color", s, prev)
		}
		seen[k] = s
	}
}

func TestICOContainer(t *testing.T) {
	p := PNG("idle")
	ico := ICO(p)
	if ico[2] != 1 || ico[4] != 1 {
		t.Fatal("bad ICO header")
	}
	off := binary.LittleEndian.Uint32(ico[18:22])
	n := binary.LittleEndian.Uint32(ico[14:18])
	if int(off)+int(n) != len(ico) || !bytes.Equal(ico[off:], p) {
		t.Fatal("bad ICO directory entry")
	}
}
