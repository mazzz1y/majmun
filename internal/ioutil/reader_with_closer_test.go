package ioutil

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestNewReaderWithCloser(t *testing.T) {
	reader := bytes.NewReader([]byte("test data"))
	closer := func() error {
		return nil
	}

	rwc := NewReaderWithCloser(reader, closer)

	if rwc.reader != reader {
		t.Error("reader not set correctly")
	}

	if rwc.closer == nil {
		t.Error("closer not set correctly")
	}
}

func TestReaderWithCloser_Read(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		bufSize    int
		expectRead int
		reader     io.Reader
	}{
		{
			name:       "normal read",
			data:       "test data",
			bufSize:    20,
			expectRead: 9,
			reader:     bytes.NewReader([]byte("test data")),
		},
		{
			name:       "partial read",
			data:       "test data",
			bufSize:    4,
			expectRead: 4,
			reader:     bytes.NewReader([]byte("test data")),
		},
		{
			name:       "empty source",
			data:       "",
			bufSize:    10,
			expectRead: 0,
			reader:     bytes.NewReader([]byte{}),
		},
		{
			name:       "nil reader",
			data:       "",
			bufSize:    10,
			expectRead: 0,
			reader:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rwc := NewReaderWithCloser(tc.reader, nil)
			buf := make([]byte, tc.bufSize)
			n, err := rwc.Read(buf)

			if tc.reader == nil {
				if !errors.Is(err, io.EOF) {
					t.Errorf("expected EOF for nil reader, got %v", err)
				}
			} else if tc.data == "" {
				if !errors.Is(err, io.EOF) {
					t.Errorf("expected EOF for empty reader, got %v", err)
				}
			}

			if n != tc.expectRead {
				t.Errorf("expected to read %d bytes, got %d", tc.expectRead, n)
			}
		})
	}
}

func TestReaderWithCloser_Close(t *testing.T) {
	tests := []struct {
		name      string
		closeErr  error
		hasClosed bool
	}{
		{
			name:      "normal close",
			closeErr:  nil,
			hasClosed: true,
		},
		{
			name:      "close with error",
			closeErr:  errors.New("close error"),
			hasClosed: true,
		},
		{
			name:      "nil closer",
			closeErr:  nil,
			hasClosed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			closeCalled := false
			var closer func() error

			if tc.hasClosed {
				closer = func() error {
					closeCalled = true
					return tc.closeErr
				}
			}

			rwc := NewReaderWithCloser(bytes.NewReader([]byte("test")), closer)
			err := rwc.Close()

			if !errors.Is(err, tc.closeErr) {
				t.Errorf("expected error %v, got %v", tc.closeErr, err)
			}

			if tc.hasClosed && !closeCalled {
				t.Error("close function was not called")
			}
		})
	}
}
