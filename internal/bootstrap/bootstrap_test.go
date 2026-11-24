package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureFileDownloadsOnce(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "sub", "model.bin")
	for i := 0; i < 2; i++ {
		if err := EnsureFile(context.Background(), srv.URL, dest, nil); err != nil {
			t.Fatal(err)
		}
	}
	st, err := os.Stat(dest)
	if err != nil || st.Size() != 5000 || hits != 1 {
		t.Fatalf("size/hits wrong: %v %d", err, hits)
	}
	if _, err := os.Stat(dest + ".part"); err == nil {
		t.Fatal("leftover .part file")
	}
}

func TestEnsureFileFailureLeavesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "m.bin")
	if err := EnsureFile(context.Background(), srv.URL, dest, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("dest must not exist after failure")
	}
}

func TestIsLocal(t *testing.T) {
	if h, p, ok := IsLocal("http://127.0.0.1:8080"); !ok || h != "127.0.0.1" || p != "8080" {
		t.Fatal("local url misparsed")
	}
	if _, _, ok := IsLocal("http://example.com:8080"); ok {
		t.Fatal("remote must not be local")
	}
}

func TestStartWhisperSkipsWhenAlreadyUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	stop, err := StartWhisper(context.Background(), "/nonexistent", "m", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	stop()
}
