//go:build darwin

package platform

import (
	"strings"
	"testing"
)

func TestAutostartPlistPlainBinary(t *testing.T) {
	p := AutostartPlist("/usr/local/bin/robot")
	if !strings.Contains(p, "<string>/usr/local/bin/robot</string>") || !strings.Contains(p, "<key>RunAtLoad</key>") {
		t.Fatal(p)
	}
}

func TestAutostartPlistBundleUsesOpen(t *testing.T) {
	p := AutostartPlist("/Applications/Robot.app/Contents/MacOS/robot")
	if !strings.Contains(p, "/usr/bin/open") || !strings.Contains(p, "<string>/Applications/Robot.app</string>") {
		t.Fatal(p)
	}
}

func TestSingleInstanceLock(t *testing.T) {
	rel, ok, err := Lock("robot-test-lock")
	if err != nil || !ok {
		t.Fatalf("first lock: ok=%v err=%v", ok, err)
	}
	_, ok2, _ := Lock("robot-test-lock")
	if ok2 {
		t.Fatal("second instance must be refused")
	}
	rel()
	rel2, ok3, _ := Lock("robot-test-lock")
	if !ok3 {
		t.Fatal("lock should be free after release")
	}
	rel2()
}
