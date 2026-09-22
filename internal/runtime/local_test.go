package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeLayer is a minimal image layer whose compressed blob is provided by rc.
type fakeLayer struct {
	rc io.ReadCloser
}

func (f fakeLayer) Compressed() (io.ReadCloser, error) { return f.rc, nil }

func gzippedTar(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// TestExtractLayerSuccess guards the normal path: a healthy layer must extract
// even with the stall watchdog enabled.
// TestExtractLayerSuccess 覆盖正常路径：启用停滞看门狗时健康的层仍应正常解压。
func TestExtractLayerSuccess(t *testing.T) {
	blob := gzippedTar(t, "etc/hello", "world")
	dest := t.TempDir()

	layer := fakeLayer{rc: io.NopCloser(bytes.NewReader(blob))}
	if err := extractLayer(layer, dest, 5*time.Second); err != nil {
		t.Fatalf("extractLayer returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "etc", "hello"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != "world" {
		t.Fatalf("content = %q, want %q", got, "world")
	}
}

// stallingReader returns an initial chunk of data, then blocks forever on
// subsequent reads until Close is called (like a network socket that stops
// delivering bytes). Close unblocks the pending read, mirroring how the real
// blob reader behaves.
// stallingReader 先返回一段数据，随后永久阻塞，直到 Close 被调用（类似停止传输
// 的网络连接）。Close 会解除阻塞，与真实 blob reader 行为一致。
type stallingReader struct {
	initial *bytes.Reader
	closed  chan struct{}
	done    bool
}

func newStallingReader(initial []byte) *stallingReader {
	return &stallingReader{
		initial: bytes.NewReader(initial),
		closed:  make(chan struct{}),
	}
}

func (s *stallingReader) Read(p []byte) (int, error) {
	if !s.done {
		n, err := s.initial.Read(p)
		if err == io.EOF {
			s.done = true
			return n, nil
		}
		return n, err
	}
	// Buffered data exhausted: block until Close, as a stalled socket would.
	// 缓冲数据耗尽后阻塞，直到 Close，模拟停止传输的网络连接。
	<-s.closed
	return 0, io.ErrClosedPipe
}

func (s *stallingReader) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

// TestExtractLayerStallAborts is the regression test for the reported hang:
// previously a layer that stopped delivering data blocked extraction forever.
// Now a stall must surface as an error within roughly the stall timeout.
// TestExtractLayerStallAborts 是针对所报告卡死的回归测试：此前停止传输的层会让解压
// 永久阻塞；现在停滞应在停滞超时左右以错误形式返回。
func TestExtractLayerStallAborts(t *testing.T) {
	// gzip header (10 bytes) so gzip.NewReader succeeds, then the stream stalls.
	blob := gzippedTar(t, "etc/hello", "world")
	layer := fakeLayer{rc: newStallingReader(blob[:10])}
	dest := t.TempDir()

	stall := 300 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- extractLayer(layer, dest, stall) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("extractLayer succeeded, want stall error")
		}
		if !strings.Contains(err.Error(), "stalled") {
			t.Fatalf("error = %v, want it to mention a network stall", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("extractLayer did not return; stall watchdog failed to fire")
	}
}

// TestStallWatchdogFires verifies the watchdog closes a blocked reader and
// reports an error after the timeout.
// TestStallWatchdogFires 验证看门狗在超时后关闭阻塞的 reader 并返回错误。
func TestStallWatchdogFires(t *testing.T) {
	sr := newStallingReader([]byte("partial"))
	w := newStallWatchdog(sr, 200*time.Millisecond)
	defer func() { _ = w.Close() }()

	buf := make([]byte, 64)
	start := time.Now()
	var lastErr error
	for {
		if _, err := w.Read(buf); err != nil {
			lastErr = err
			break
		}
		if time.Since(start) > 3*time.Second {
			t.Fatal("watchdog never returned an error")
		}
	}
	if !strings.Contains(lastErr.Error(), "stalled") {
		t.Fatalf("error = %v, want stall error", lastErr)
	}
}

// TestStallWatchdogDisabled confirms a non-positive timeout leaves the reader
// untouched so callers can opt out.
// TestStallWatchdogDisabled 确认非正超时不会包装 reader，调用方可选择退出。
func TestStallWatchdogDisabled(t *testing.T) {
	blob := gzippedTar(t, "etc/hello", "world")
	layer := fakeLayer{rc: io.NopCloser(bytes.NewReader(blob))}
	dest := t.TempDir()
	if err := extractLayer(layer, dest, 0); err != nil {
		t.Fatalf("extractLayer with disabled stall timeout: %v", err)
	}
}
