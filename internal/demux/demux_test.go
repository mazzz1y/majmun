package demux

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
)

const (
	concurrentClients  = 100
	rapidConnectCycles = 500
	largeDataSize      = 1024 * 1024
	streamChunks       = 100
	chunkSize          = 4096
)

type mockStreamer struct {
	data       []byte
	chunks     int
	chunkSize  int
	chunkDelay time.Duration
	writeErr   error
}

func (m *mockStreamer) Stream(ctx context.Context, w io.Writer) (int64, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}

	var total int64

	if m.data != nil {
		n, err := w.Write(m.data)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}

	if m.chunks > 0 {
		chunk := make([]byte, m.chunkSize)
		for i := range m.chunks {
			for j := range chunk {
				chunk[j] = byte(i ^ j)
			}
			n, err := w.Write(chunk)
			total += int64(n)
			if err != nil {
				return total, err
			}
			if m.chunkDelay > 0 {
				select {
				case <-time.After(m.chunkDelay):
				case <-ctx.Done():
					return total, ctx.Err()
				}
			}
		}
	}

	<-ctx.Done()
	return total, ctx.Err()
}

func TestGetReader_SingleClient(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := []byte("hello world")
	req := Request{
		StreamKey: "test-stream",
		Streamer:  &mockStreamer{data: data},
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, len(data))
	n, err := io.ReadFull(reader, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if n != len(data) || !bytes.Equal(buf, data) {
		t.Errorf("got %q, want %q", buf[:n], data)
	}
}

func TestGetReader_MultipleClients(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := []byte("shared data")
	req := Request{
		StreamKey: "test-stream",
		Streamer:  &mockStreamer{data: data},
	}

	reader1, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 1 failed: %v", err)
	}
	defer reader1.Close()

	reader2, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 2 failed: %v", err)
	}
	defer reader2.Close()

	var wg sync.WaitGroup
	results := make([][]byte, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, len(data))
		io.ReadFull(reader1, buf)
		results[0] = buf
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, len(data))
		io.ReadFull(reader2, buf)
		results[1] = buf
	}()

	wg.Wait()

	if !bytes.Equal(results[0], data) {
		t.Errorf("reader1 got %q, want %q", results[0], data)
	}
	if !bytes.Equal(results[1], data) {
		t.Errorf("reader2 got %q, want %q", results[1], data)
	}
}

func TestGetReader_ClientDisconnect(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	req := Request{
		StreamKey: "test-stream",
		Streamer:  &mockStreamer{},
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}

	cancel()

	time.Sleep(50 * time.Millisecond)

	_, err = reader.Read(make([]byte, 10))
	if err == nil {
		t.Error("expected error after context cancel")
	}
}

func TestGetReader_SemaphoreLimit(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	sem := semaphore.NewWeighted(1)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	req1 := Request{
		StreamKey: "stream-1",
		Streamer:  &mockStreamer{},
		Semaphore: sem,
	}

	_, err := d.GetReader(ctx1, req1)
	if err != nil {
		t.Fatalf("GetReader 1 failed: %v", err)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	req2 := Request{
		StreamKey: "stream-2",
		Streamer:  &mockStreamer{},
		Semaphore: sem,
	}

	_, err = d.GetReader(ctx2, req2)
	if err != ErrSubscriptionSemaphore {
		t.Errorf("expected ErrSubscriptionSemaphore, got %v", err)
	}
}

func TestGetReader_SemaphoreReleased(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	sem := semaphore.NewWeighted(1)

	ctx1, cancel1 := context.WithCancel(context.Background())

	req := Request{
		StreamKey: "test-stream",
		Streamer:  &mockStreamer{},
		Semaphore: sem,
	}

	reader1, err := d.GetReader(ctx1, req)
	if err != nil {
		t.Fatalf("GetReader 1 failed: %v", err)
	}

	cancel1()
	reader1.Close()

	time.Sleep(100 * time.Millisecond)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	req.StreamKey = "test-stream-2"
	_, err = d.GetReader(ctx2, req)
	if err != nil {
		t.Errorf("GetReader 2 failed after semaphore release: %v", err)
	}
}

func TestGetReader_ConcurrentAccess(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, concurrentClients)

	for range concurrentClients {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			req := Request{
				StreamKey: "shared-stream",
				Streamer:  &mockStreamer{data: []byte("test")},
			}

			reader, err := d.GetReader(ctx, req)
			if err != nil {
				errors <- err
				return
			}

			time.Sleep(10 * time.Millisecond)
			reader.Close()
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}
}

func TestGetReader_RapidConnectDisconnect(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	for i := range rapidConnectCycles {
		ctx, cancel := context.WithCancel(context.Background())

		req := Request{
			StreamKey: "test-stream",
			Streamer:  &mockStreamer{data: []byte("x")},
		}

		reader, err := d.GetReader(ctx, req)
		if err != nil {
			t.Fatalf("iteration %d: GetReader failed: %v", i, err)
		}

		cancel()
		reader.Close()

		time.Sleep(5 * time.Millisecond)
	}
}

func TestGetReader_LargeData(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := make([]byte, largeDataSize)
	for i := range data {
		data[i] = byte(i)
	}

	req := Request{
		StreamKey: "test-stream",
		Streamer:  &mockStreamer{data: data},
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, largeDataSize)
	n, err := io.ReadFull(reader, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if n != largeDataSize {
		t.Errorf("got %d bytes, want %d", n, largeDataSize)
	}

	if !bytes.Equal(buf, data) {
		t.Error("data mismatch")
	}
}

func TestGetReader_StreamingData(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := Request{
		StreamKey: "test-stream",
		Streamer: &mockStreamer{
			chunks:     streamChunks,
			chunkSize:  chunkSize,
			chunkDelay: time.Millisecond,
		},
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}
	defer reader.Close()

	expectedSize := streamChunks * chunkSize
	buf := make([]byte, expectedSize)
	n, err := io.ReadFull(reader, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if n != expectedSize {
		t.Errorf("got %d bytes, want %d", n, expectedSize)
	}

	for i := range streamChunks {
		offset := i * chunkSize
		for j := range chunkSize {
			expected := byte(i ^ j)
			if buf[offset+j] != expected {
				t.Fatalf("data mismatch at chunk %d offset %d: got %d, want %d", i, j, buf[offset+j], expected)
			}
		}
	}
}

func TestGetReader_LateJoinerGetsBuffer(t *testing.T) {
	d := NewDemuxer()
	defer d.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := Request{
		StreamKey: "test-stream",
		Streamer: &mockStreamer{
			chunks:     streamChunks,
			chunkSize:  chunkSize,
			chunkDelay: time.Millisecond,
		},
	}

	reader1, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 1 failed: %v", err)
	}
	defer reader1.Close()

	time.Sleep(50 * time.Millisecond)

	reader2, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader 2 failed: %v", err)
	}
	defer reader2.Close()

	buf := make([]byte, 1024)
	n, err := reader2.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if n == 0 {
		t.Error("late joiner should receive buffered data")
	}
}

func TestStop(t *testing.T) {
	d := NewDemuxer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := Request{
		StreamKey: "test-stream",
		Streamer:  &mockStreamer{},
	}

	reader, err := d.GetReader(ctx, req)
	if err != nil {
		t.Fatalf("GetReader failed: %v", err)
	}

	d.Stop()

	_, err = reader.Read(make([]byte, 10))
	if err == nil {
		t.Error("expected error after Stop")
	}
}
