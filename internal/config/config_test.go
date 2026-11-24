package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.WakeWord != "робот" || c.SilenceMS != 1200 || c.MaxToolCalls != 6 {
		t.Fatalf("bad defaults: %+v", c)
	}
}

func TestUpdateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	c, _ := Load(p)
	if err := c.Update(func(c *Config) { c.WakeWord = "компьютер"; c.Autostart = true }); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c2.WakeWord != "компьютер" || !c2.Autostart || c2.FollowUpSeconds != 8 {
		t.Fatalf("round trip failed: %+v", c2)
	}
}
