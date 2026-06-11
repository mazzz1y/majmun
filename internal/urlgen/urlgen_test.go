package urlgen

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGenerator_CreateURL(t *testing.T) {
	g, err := NewGenerator("https://example.com", "test-secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	tests := []struct {
		name    string
		data    Data
		ttl     time.Duration
		wantExt string
		wantErr bool
	}{
		{
			name: "stream request",
			data: Data{
				RequestType: RequestTypeStream,
				StreamData: StreamData{
					ChannelName: "1",
					Streams: []Stream{{
						URL:    "https://stream.example.com/video",
						Hidden: false,
					}},
				},
			},
			ttl:     time.Hour,
			wantExt: ".ts",
			wantErr: false,
		},
		{
			name: "file request with extension",
			data: Data{
				RequestType: RequestTypeFile,
				File:        FileData{URL: "https://file.example.com/video.mp4"},
			},
			ttl:     time.Hour,
			wantExt: ".mp4",
			wantErr: false,
		},
		{
			name: "file request without extension",
			data: Data{
				RequestType: RequestTypeFile,
				File:        FileData{URL: "https://file.example.com/video"},
			},
			ttl:     time.Hour,
			wantExt: ".file",
			wantErr: false,
		},
		{
			name: "file request with query params",
			data: Data{
				RequestType: RequestTypeFile,
				File:        FileData{URL: "https://file.example.com/video.mp4?token=abc123"},
			},
			ttl:     time.Hour,
			wantExt: ".mp4",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := g.CreateURL(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("createURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if !strings.HasPrefix(u.String(), g.PublicURL) {
				t.Errorf("createURL() url doesn't start with public url: %v", u.String())
			}

			if !strings.HasSuffix(u.Path, "/f"+tt.wantExt) {
				t.Errorf("createURL() url doesn't end with expected extension: got %v, want %v", u.Path, "/f"+tt.wantExt)
			}

			parts := strings.Split(u.Path, "/")
			if len(parts) < 2 {
				t.Error("createURL() invalid url format")
				return
			}
			token := parts[len(parts)-2]

			decrypted, err := g.Decrypt(token)
			if err != nil {
				t.Errorf("failed to decrypt token: %v", err)
				return
			}

			if decrypted.RequestType != tt.data.RequestType {
				t.Errorf("decrypted requestType = %v, want %v", decrypted.RequestType, tt.data.RequestType)
			}

			if tt.data.RequestType == RequestTypeStream {
				if decrypted.StreamData.ChannelName != tt.data.StreamData.ChannelName {
					t.Errorf("decrypted channelID = %v, want %v", decrypted.StreamData.ChannelName, tt.data.StreamData.ChannelName)
				}
				if len(decrypted.StreamData.Streams) != len(tt.data.StreamData.Streams) {
					t.Errorf("decrypted streams length = %v, want %v", len(decrypted.StreamData.Streams), len(tt.data.StreamData.Streams))
				}
				for i, stream := range tt.data.StreamData.Streams {
					if decrypted.StreamData.Streams[i].URL != stream.URL {
						t.Errorf("decrypted stream[%d].URL = %v, want %v", i, decrypted.StreamData.Streams[i].URL, stream.URL)
					}
					if decrypted.StreamData.Streams[i].Hidden != stream.Hidden {
						t.Errorf("decrypted stream[%d].Hidden = %v, want %v", i, decrypted.StreamData.Streams[i].Hidden, stream.Hidden)
					}
				}
			} else {
				if decrypted.File.URL != tt.data.File.URL {
					t.Errorf("decrypted file.URL = %v, want %v", decrypted.File.URL, tt.data.File.URL)
				}
			}
		})
	}
}

func TestGenerator_CreateStreamURL(t *testing.T) {
	g, err := NewGenerator("https://example.com", "test-secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	streams := []Stream{
		{
			ProviderInfo: ProviderInfo{ProviderType: ProviderTypePlaylist, ProviderID: "test1"},
			URL:          "https://stream1.example.com/video",
			Hidden:       false,
		},
		{
			ProviderInfo: ProviderInfo{ProviderType: ProviderTypePlaylist, ProviderID: "test2"},
			URL:          "https://stream2.example.com/video",
			Hidden:       true,
		},
	}

	u, err := g.CreateStreamURL("1", streams)
	if err != nil {
		t.Fatalf("CreateStreamURL() failed: %v", err)
	}

	if !strings.HasPrefix(u.String(), g.PublicURL) {
		t.Errorf("CreateStreamURL() URL doesn't start with public URL")
	}

	if !strings.HasSuffix(u.Path, "/f.ts") {
		t.Errorf("CreateStreamURL() URL doesn't end with .ts extension")
	}

	parts := strings.Split(u.Path, "/")
	token := parts[len(parts)-2]

	decrypted, err := g.Decrypt(token)
	if err != nil {
		t.Fatalf("Failed to decrypt token: %v", err)
	}

	if decrypted.RequestType != RequestTypeStream {
		t.Errorf("Expected RequestTypeStream, got %v", decrypted.RequestType)
	}

	if decrypted.StreamData.ChannelName != "1" {
		t.Errorf("Expected channelID 1, got %s", decrypted.StreamData.ChannelName)
	}

	if len(decrypted.StreamData.Streams) != 2 {
		t.Errorf("Expected 2 streams, got %d", len(decrypted.StreamData.Streams))
	}

	for i, expected := range streams {
		if decrypted.StreamData.Streams[i] != expected {
			t.Errorf("Stream %d mismatch: got %+v, want %+v", i, decrypted.StreamData.Streams[i], expected)
		}
	}
}

func TestGenerator_CreateFileURL(t *testing.T) {
	g, err := NewGenerator("https://example.com", "test-secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	fileURL := "https://file.example.com/document.pdf"

	providerInfo := ProviderInfo{ProviderType: ProviderTypePlaylist, ProviderID: "test"}
	u, err := g.CreateFileURL(providerInfo, fileURL)
	if err != nil {
		t.Fatalf("CreateFileURL() failed: %v", err)
	}

	if !strings.HasPrefix(u.String(), g.PublicURL) {
		t.Errorf("CreateFileURL() URL doesn't start with public URL")
	}

	if !strings.HasSuffix(u.Path, "/f.pdf") {
		t.Errorf("CreateFileURL() URL doesn't end with .pdf extension")
	}

	parts := strings.Split(u.Path, "/")
	token := parts[len(parts)-2]

	decrypted, err := g.Decrypt(token)
	if err != nil {
		t.Fatalf("Failed to decrypt token: %v", err)
	}

	if decrypted.RequestType != RequestTypeFile {
		t.Errorf("Expected RequestTypeFile, got %v", decrypted.RequestType)
	}

	if decrypted.File.URL != fileURL {
		t.Errorf("Expected File.URL %s, got %s", fileURL, decrypted.File.URL)
	}
}

func TestGenerator_Decrypt(t *testing.T) {
	g, err := NewGenerator("https://example.com", "test-secret", time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	validData := Data{
		RequestType: RequestTypeStream,
		StreamData: StreamData{
			ChannelName: "1",
			Streams: []Stream{{
				URL:    "https://example.com/video",
				Hidden: false,
			}},
		},
	}

	u, err := g.CreateURL(validData)
	if err != nil {
		t.Fatalf("Failed to create URL: %v", err)
	}

	parts := strings.Split(u.Path, "/")
	validToken := parts[len(parts)-2]

	tests := []struct {
		name    string
		token   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid token",
			token:   validToken,
			wantErr: false,
		},
		{
			name:    "invalid base64",
			token:   "invalid!@#$%",
			wantErr: true,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "short token",
			token:   "dG9v",
			wantErr: true,
			errMsg:  "invalid token",
		},
		{
			name:    "tampered token",
			token:   validToken[:len(validToken)-4] + "XXXX",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := g.Decrypt(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("decrypt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("decrypt() error = %v, want error containing %v", err, tt.errMsg)
			}
			if !tt.wantErr && data == nil {
				t.Error("decrypt() returned nil data")
			}
		})
	}
}

func TestGenerator_ExpiredLinkDecryption(t *testing.T) {
	t.Run("expired token with TTL", func(t *testing.T) {
		g, err := NewGenerator("https://example.com", "test-secret", time.Millisecond, time.Millisecond)
		if err != nil {
			t.Fatalf("Failed to create generator: %v", err)
		}

		data := Data{
			RequestType: RequestTypeStream,
			StreamData: StreamData{
				ChannelName: "1",
				Streams: []Stream{{
					URL:    "https://example.com/video",
					Hidden: false,
				}},
			},
			CreatedAt: time.Now().Add(-10 * time.Millisecond).Unix(),
		}

		u, err := g.CreateURL(data)
		if err != nil {
			t.Fatalf("Failed to create URL: %v", err)
		}

		parts := strings.Split(u.Path, "/")
		token := parts[len(parts)-2]

		_, err = g.Decrypt(token)
		if err == nil {
			t.Error("expected error when decrypting expired token")
		}

		if !errors.Is(err, ErrExpiredStreamURL) {
			t.Errorf("expected ErrExpiredStreamURL, got %v", err)
		}
	})

	t.Run("valid token with zero TTL", func(t *testing.T) {
		g, err := NewGenerator("https://example.com", "test-secret", 0, 0)
		if err != nil {
			t.Fatalf("Failed to create generator: %v", err)
		}

		data := Data{
			RequestType: RequestTypeStream,
			StreamData: StreamData{
				ChannelName: "1",
				Streams: []Stream{{
					URL:    "https://example.com/video",
					Hidden: false,
				}},
			},
			CreatedAt: time.Now().Add(-time.Hour).Unix(),
		}

		u, err := g.CreateURL(data)
		if err != nil {
			t.Fatalf("Failed to create URL: %v", err)
		}

		parts := strings.Split(u.Path, "/")
		token := parts[len(parts)-2]

		_, err = g.Decrypt(token)
		if err != nil {
			t.Errorf("expected no error for zero TTL, got %v", err)
		}
	})
}

func TestDetermineExtension(t *testing.T) {
	tests := []struct {
		name string
		data Data
		want string
	}{
		{
			name: "stream type",
			data: Data{RequestType: RequestTypeStream},
			want: ".ts",
		},
		{
			name: "file with .mp4",
			data: Data{RequestType: RequestTypeFile, File: FileData{URL: "https://example.com/video.mp4"}},
			want: ".mp4",
		},
		{
			name: "file with .m3u8",
			data: Data{RequestType: RequestTypeFile, File: FileData{URL: "https://example.com/playlist.m3u8"}},
			want: ".m3u8",
		},
		{
			name: "file with query params",
			data: Data{RequestType: RequestTypeFile, File: FileData{URL: "https://example.com/video.mp4?token=123"}},
			want: ".mp4",
		},
		{
			name: "file without extension",
			data: Data{RequestType: RequestTypeFile, File: FileData{URL: "https://example.com/video"}},
			want: ".file",
		},
		{
			name: "file with multiple dots",
			data: Data{RequestType: RequestTypeFile, File: FileData{URL: "https://example.com/my.video.file.mp4"}},
			want: ".mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := determineExtension(tt.data); got != tt.want {
				t.Errorf("determineExtension() = %v, want %v", got, tt.want)
			}
		})
	}
}
