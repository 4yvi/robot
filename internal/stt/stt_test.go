package stt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribe(t *testing.T) {
	var gotLang, gotFile string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference" {
			t.Errorf("path %s", r.URL.Path)
		}
		_ = r.ParseMultipartForm(1 << 20)
		gotLang = r.FormValue("language")
		f, _, err := r.FormFile("file")
		if err == nil {
			b := make([]byte, 4)
			_, _ = f.Read(b)
			gotFile = string(b)
		}
		_, _ = w.Write([]byte(`{"text":" Привет, мир. [BLANK_AUDIO]\n"}`))
	}))
	defer srv.Close()
	text, err := New(srv.URL+"/", "ru").Transcribe(context.Background(), []byte("RIFFdata"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "Привет, мир." || gotLang != "ru" || gotFile != "RIFF" {
		t.Fatalf("text=%q lang=%q file=%q", text, gotLang, gotFile)
	}
}

func TestTranscribeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "").Transcribe(context.Background(), nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestClean(t *testing.T) {
	if got := Clean("  [музыка] да (смех) нет  "); got != "да нет" {
		t.Fatalf("got %q", got)
	}
}
