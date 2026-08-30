package ioutil

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
)

type mockReadCloser struct {
	data     string
	pos      int
	closed   bool
	readErr  error
	closeErr error
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return m.closeErr
}

func TestCountReadCloser_Read(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		bufSize    int
		expectRead int
		readErr    error
	}{
		{
			name:       "normal read",
			source:     "test data",
			bufSize:    20,
			expectRead: 9,
		},
		{
			name:       "partial read",
			source:     "test data",
			bufSize:    4,
			expectRead: 4,
		},
		{
			name:       "empty source",
			source:     "",
			bufSize:    10,
			expectRead: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &mockReadCloser{data: tc.source, readErr: tc.readErr}
			var counter atomic.Int64
			reader := NewCountReadCloser(src, &counter)
			buf := make([]byte, tc.bufSize)
			n, err := reader.Read(buf)

			if tc.readErr != nil && !errors.Is(err, tc.readErr) {
				t.Errorf("expected error %v, got %v", tc.readErr, err)
			}

			if n != tc.expectRead {
				t.Errorf("expected to read %d bytes, got %d", tc.expectRead, n)
			}

			if counter.Load() != int64(tc.expectRead) {
				t.Errorf("expected counter to be %d, got %d", tc.expectRead, counter.Load())
			}
		})
	}
}

func TestCountReadCloser_ReadWithNilSource(t *testing.T) {
	reader := NewCountReadCloser(nil, nil)
	buf := make([]byte, 10)
	n, err := reader.Read(buf)

	if n != 0 {
		t.Errorf("expected to read 0 bytes, got %d", n)
	}

	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF error, got %v", err)
	}
}

func TestCountReadCloser_ReadWithNilCounter(t *testing.T) {
	src := &mockReadCloser{data: "test data"}
	reader := NewCountReadCloser(src, nil)
	buf := make([]byte, 10)
	n, err := reader.Read(buf)

	if n != 9 {
		t.Errorf("expected to read 9 bytes, got %d", n)
	}

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCountReadCloser_Close(t *testing.T) {
	tests := []struct {
		name     string
		src      io.ReadCloser
		closeErr error
	}{
		{
			name: "normal close",
			src:  &mockReadCloser{},
		},
		{
			name:     "close with error",
			src:      &mockReadCloser{closeErr: io.ErrClosedPipe},
			closeErr: io.ErrClosedPipe,
		},
		{
			name: "nil source",
			src:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := NewCountReadCloser(tc.src, nil)
			err := reader.Close()

			if !errors.Is(err, tc.closeErr) {
				t.Errorf("expected error %v, got %v", tc.closeErr, err)
			}

			if tc.src != nil {
				if mockSrc, ok := tc.src.(*mockReadCloser); ok && !mockSrc.closed {
					t.Error("source was not closed")
				}
			}
		})
	}
}
