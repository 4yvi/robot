// Package bootstrap prepares what the assistant needs at first run: it
// downloads the large speech model and starts the local whisper-server.
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// EnsureFile downloads srcURL to dest unless dest already exists. The file is
// written to dest+".part" first so an interrupted download is never mistaken
// for a complete one. progress may be nil.
func EnsureFile(ctx context.Context, srcURL, dest string, progress func(done, total int64)) error {
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", srcURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", srcURL, resp.Status)
	}
	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, fn: progress}
	_, err = io.Copy(f, pr)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(part)
		return fmt.Errorf("download %s: %w", srcURL, err)
	}
	if pr.total > 0 && pr.done != pr.total {
		os.Remove(part)
		return fmt.Errorf("download %s: incomplete (%d of %d bytes)", srcURL, pr.done, pr.total)
	}
	return os.Rename(part, dest)
}

type progressReader struct {
	r           io.Reader
	done, total int64
	fn          func(done, total int64)
	last        time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.fn != nil && time.Since(p.last) > time.Second {
		p.last = time.Now()
		p.fn(p.done, p.total)
	}
	return n, err
}

// Reachable reports whether something answers HTTP at base.
func Reachable(base string) bool {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get(base)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// IsLocal reports whether the URL points at this machine.
func IsLocal(base string) (host, port string, ok bool) {
	u, err := url.Parse(base)
	if err != nil {
		return "", "", false
	}
	h := u.Hostname()
	if h != "localhost" && h != "127.0.0.1" && h != "::1" {
		return "", "", false
	}
	p := u.Port()
	if p == "" {
		p = "80"
	}
	return h, p, true
}

// StartWhisper launches whisper-server for the given model unless something
// already listens at baseURL, or the URL is not local. The returned stop
// function terminates the child (it is a no-op when nothing was started).
func StartWhisper(ctx context.Context, serverPath, modelPath, baseURL string) (stop func(), err error) {
	noop := func() {}
	host, port, local := IsLocal(baseURL)
	if !local || Reachable(baseURL) {
		return noop, nil
	}
	if _, err := os.Stat(serverPath); err != nil {
		return noop, fmt.Errorf("whisper-server not found at %s (start it yourself or fix whisper_server_path)", serverPath)
	}
	if host == "::1" {
		host = "127.0.0.1"
	}
	cmd := exec.Command(serverPath, "-m", modelPath, "-l", "ru", "--host", host, "--port", port)
	cmd.Dir = filepath.Dir(serverPath)
	hide(cmd)
	if err := cmd.Start(); err != nil {
		return noop, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	stop = func() {
		_ = cmd.Process.Kill()
		<-exited
	}
	addr := net.JoinHostPort(host, port)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			stop()
			return noop, ctx.Err()
		case <-exited:
			return noop, fmt.Errorf("whisper-server exited right after start")
		case <-time.After(300 * time.Millisecond):
		}
		if c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond); err == nil {
			c.Close()
			return stop, nil
		}
	}
	stop()
	return noop, fmt.Errorf("whisper-server did not start within 60 s")
}
