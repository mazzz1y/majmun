package streampool

import (
	"context"
	"io"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
)

var testSegmenterCfg = proxy.Segmenter{
	Command: common.StringOrArr{
		"ffmpeg",
		"-v", "fatal",
		"-i", "pipe:0",
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "{{ .segment_duration }}",
		"-hls_list_size", "{{ .max_segments }}",
		"-hls_flags", "delete_segments+append_list+independent_segments",
		"-hls_segment_filename", "{{ .segment_path }}",
		"{{ .playlist_path }}",
	},
	TemplateVars: []common.NameValue{
		{Name: "segment_duration", Value: "2"},
		{Name: "max_segments", Value: "15"},
	},
	InitSegments: intPtr(1),
	ReadyTimeout: durationPtr(30 * time.Second),
}

func intPtr(i int) *int { return &i }
func durationPtr(d time.Duration) *common.Duration {
	cd := common.Duration(d)
	return &cd
}

func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func skipWithoutFFmpeg(t *testing.T) {
	t.Helper()
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
}

type mockStreamer struct {
	duration time.Duration
}

func (m *mockStreamer) RunWithStdout(ctx context.Context, w io.Writer) (int64, error) {
	dur := m.duration
	if dur == 0 {
		dur = 30 * time.Second
	}

	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-v", "fatal",
		"-f", "lavfi",
		"-i", "testsrc=duration=60:size=320x240:rate=10",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-g", "10",
		"-f", "mpegts",
		"-t", dur.String(),
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}

	if err := cmd.Start(); err != nil {
		return 0, err
	}

	var total int64
	buf := make([]byte, 64*1024)
	for ctx.Err() == nil {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			written, writeErr := w.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				break
			}
		}
		if readErr != nil {
			break
		}
	}

	_ = cmd.Wait()
	return total, ctx.Err()
}

func TestSegmenterPool_CreateAndRemove(t *testing.T) {
	pool := newSegmenterPool()
	ctx := context.Background()
	dir := t.TempDir()

	seg1, isNew, err := pool.getOrCreate("stream-1", ctx, dir, testSegmenterCfg)
	if err != nil {
		t.Fatalf("getOrCreate failed: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true for first creation")
	}
	if seg1 == nil {
		t.Fatal("expected non-nil segmenter")
	}

	seg2, isNew2, err := pool.getOrCreate("stream-1", ctx, dir, testSegmenterCfg)
	if err != nil {
		t.Fatalf("getOrCreate failed: %v", err)
	}
	if isNew2 {
		t.Error("expected isNew=false for existing stream")
	}
	if seg1 != seg2 {
		t.Error("expected same segmenter instance")
	}

	pool.remove("stream-1")

	seg3, isNew3, err := pool.getOrCreate("stream-1", ctx, dir, testSegmenterCfg)
	if err != nil {
		t.Fatalf("getOrCreate failed: %v", err)
	}
	if !isNew3 {
		t.Error("expected isNew=true after removal")
	}
	if seg3 == seg1 {
		t.Error("expected new segmenter instance after removal")
	}
}

func TestSegmenterPool_StopAllClearsMap(t *testing.T) {
	pool := newSegmenterPool()
	ctx := context.Background()
	dir := t.TempDir()

	_, _, _ = pool.getOrCreate("stream-1", ctx, dir, testSegmenterCfg)
	_, _, _ = pool.getOrCreate("stream-2", ctx, dir, testSegmenterCfg)

	pool.stopAll()

	pool.mu.Lock()
	count := len(pool.segmenters)
	pool.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 segmenters after stopAll, got %d", count)
	}
}

func TestSegmenter_InitialClientCountIsOne(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seg, err := newSegmenter(ctx, "test", t.TempDir(), testSegmenterCfg)
	if err != nil {
		t.Fatalf("newSegmenter failed: %v", err)
	}

	if seg.clientCount.Load() != 1 {
		t.Errorf("expected 1 initial client, got %d", seg.clientCount.Load())
	}
}

func TestSegmenter_EmptySignalOnLastClientRemoved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seg, err := newSegmenter(ctx, "test", t.TempDir(), testSegmenterCfg)
	if err != nil {
		t.Fatalf("newSegmenter failed: %v", err)
	}

	seg.addClient()
	seg.addClient()

	if seg.clientCount.Load() != 3 {
		t.Errorf("expected 3 clients, got %d", seg.clientCount.Load())
	}

	seg.removeClient()
	seg.removeClient()

	select {
	case <-seg.waitEmpty():
		t.Error("should not be empty with 1 client remaining")
	default:
	}

	seg.removeClient()

	select {
	case <-seg.waitEmpty():
	case <-time.After(100 * time.Millisecond):
		t.Error("expected empty signal after all clients removed")
	}
}

func TestSegmenter_DirCreatedOnInit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	baseDir := t.TempDir()
	_, err := newSegmenter(ctx, "test-stream", baseDir, testSegmenterCfg)
	if err != nil {
		t.Fatalf("newSegmenter failed: %v", err)
	}

	expectedDir := filepath.Join(baseDir, "test-stream")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Error("expected segment directory to exist")
	}
}

func TestSegmenter_CleanupRemovesDir(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	baseDir := t.TempDir()
	seg, err := newSegmenter(ctx, "test-stream", baseDir, testSegmenterCfg)
	if err != nil {
		t.Fatalf("newSegmenter failed: %v", err)
	}

	seg.cleanup()

	expectedDir := filepath.Join(baseDir, "test-stream")
	if _, err := os.Stat(expectedDir); !os.IsNotExist(err) {
		t.Error("expected segment directory to be removed")
	}
}

func TestGetReader_SingleClientReceivesData(t *testing.T) {
	skipWithoutFFmpeg(t)

	d := New()
	defer d.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		StreamKey: "test-single",
		Streamer:  &mockStreamer{duration: 15 * time.Second},
		Segmenter: testSegmenterCfg,
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	buf := make([]byte, 64*1024)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n == 0 {
		t.Error("expected non-zero bytes from reader")
	}
}

func TestGetReader_TwoClientsShareOneSegmenter(t *testing.T) {
	skipWithoutFFmpeg(t)

	d := New()
	defer d.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		StreamKey: "test-multi",
		Streamer:  &mockStreamer{duration: 15 * time.Second},
		Segmenter: testSegmenterCfg,
	}

	reader1, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 1 failed: %v", err)
	}
	defer func() { _ = reader1.Close() }()

	reader2, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 2 failed: %v", err)
	}
	defer func() { _ = reader2.Close() }()

	var wg sync.WaitGroup
	bytesRead := make([]int, 2)

	for i, reader := range []io.ReadCloser{reader1, reader2} {
		wg.Add(1)
		go func(idx int, r io.ReadCloser) {
			defer wg.Done()
			buf := make([]byte, 64*1024)
			n, _ := r.Read(buf)
			bytesRead[idx] = n
		}(i, reader)
	}

	wg.Wait()

	if bytesRead[0] == 0 {
		t.Error("reader1 got 0 bytes")
	}
	if bytesRead[1] == 0 {
		t.Error("reader2 got 0 bytes")
	}
}

func TestGetReader_ReadFailsAfterClose(t *testing.T) {
	skipWithoutFFmpeg(t)

	d := New()
	defer d.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		StreamKey: "test-disconnect",
		Streamer:  &mockStreamer{duration: 15 * time.Second},
		Segmenter: testSegmenterCfg,
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	_, err = reader.Read(buf)
	if err != nil {
		t.Fatalf("initial Read failed: %v", err)
	}

	_ = reader.Close()

	_, err = reader.Read(buf)
	if err == nil {
		t.Error("expected error after Close")
	}
}

func TestGetReader_SemaphoreBlocksSecondStream(t *testing.T) {
	skipWithoutFFmpeg(t)

	d := New()
	defer d.Stop()

	sem := semaphore.NewWeighted(1)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()

	req1 := Request{
		StreamKey: "sem-stream-1",
		Streamer:  &mockStreamer{duration: 15 * time.Second},
		Semaphore: sem,
		Segmenter: testSegmenterCfg,
	}

	reader1, err := d.GetReader(ctx1, req1)
	if err != nil {
		t.Fatalf("GetReader 1 failed: %v", err)
	}
	defer func() { _ = reader1.Close() }()

	go func() { _, _ = io.Copy(io.Discard, reader1) }()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	req2 := Request{
		StreamKey: "sem-stream-2",
		Streamer:  &mockStreamer{duration: 15 * time.Second},
		Semaphore: sem,
		Segmenter: testSegmenterCfg,
	}

	_, err = d.GetReader(ctx2, req2)
	if err != ErrSubscriptionSemaphore {
		t.Errorf("expected ErrSubscriptionSemaphore, got %v", err)
	}
}

func TestGetReader_JoiningExistingStreamDoesNotConsumeSemaphore(t *testing.T) {
	skipWithoutFFmpeg(t)

	d := New()
	defer d.Stop()

	sem := semaphore.NewWeighted(1)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		StreamKey: "sem-shared",
		Streamer:  &mockStreamer{duration: 15 * time.Second},
		Semaphore: sem,
		Segmenter: testSegmenterCfg,
	}

	reader1, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 1 failed: %v", err)
	}
	defer func() { _ = reader1.Close() }()

	reader2, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 2 failed: %v", err)
	}
	defer func() { _ = reader2.Close() }()
}

func TestStop_ReaderFailsAfterPoolStopped(t *testing.T) {
	skipWithoutFFmpeg(t)

	d := New()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		StreamKey: "test-stop",
		Streamer:  &mockStreamer{duration: 15 * time.Second},
		Segmenter: testSegmenterCfg,
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}

	d.Stop()

	_, err = reader.Read(make([]byte, 1024))
	if err == nil {
		t.Error("expected error after Stop")
	}
}
