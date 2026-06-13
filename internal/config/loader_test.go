package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		expectError   bool
		validate      func(t *testing.T, cfg *Config)
	}{
		{
			name: "valid minimal config",
			configContent: `server:
  listen_addr: ":8080"
  public_url: "http://example.com"
url_generator:
  secret: "test-secret"`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.ListenAddr != ":8080" {
					t.Errorf("expected ListenAddr to be ':8080', got '%s'", cfg.Server.ListenAddr)
				}
				if cfg.Server.PublicURL.String() != "http://example.com" {
					t.Errorf("expected PublicURL to be 'http://example.com', got '%s'", cfg.Server.PublicURL.String())
				}
				if cfg.URLGenerator.Secret != "test-secret" {
					t.Errorf("expected Secret to be 'test-secret', got '%s'", cfg.URLGenerator.Secret)
				}
			},
		},
		{
			name:          "file not found",
			configContent: "",
			expectError:   true,
			validate:      nil,
		},
		{
			name:          "invalid yaml",
			configContent: `listen_addr: {invalid-yaml`,
			expectError:   true,
			validate:      nil,
		},
		{
			name:          "invalid public URL",
			configContent: "server:\n  listen_addr: \":8080\"\n  public_url: \"://invalid\"\nurl_generator:\n  secret: \"test-secret\"",
			expectError:   true,
			validate:      nil,
		},
		{
			name: "directory with multiple files",
			configContent: `server:
  listen_addr: ":8080"
  public_url: "http://example.com"
url_generator:
  secret: "test-secret"`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.ListenAddr != ":8080" {
					t.Errorf("expected ListenAddr to be ':8080', got '%s'", cfg.Server.ListenAddr)
				}
			},
		},
		{
			name: "top-level playout with channel override parses",
			configContent: `server:
  listen_addr: ":8080"
  public_url: "http://example.com"
url_generator:
  secret: "test-secret"
playout:
  schedule_swap_at: "03:00"
  template_variables:
    - name: video_bitrate_kbps
      value: "3000"
playlists:
  - name: local
    channels:
      - name: movies
        sources: [/media/movies]
        playout:
          template_variables:
            - name: video_bitrate_kbps
              value: "6000"`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Playout.ScheduleSwapAt == nil || *cfg.Playout.ScheduleSwapAt != "03:00" {
					t.Errorf("expected global schedule_swap_at 03:00, got %v", cfg.Playout.ScheduleSwapAt)
				}
				if len(cfg.Playout.TemplateVars) <= 1 {
					t.Errorf("expected default template vars to survive merge with user var, got %d", len(cfg.Playout.TemplateVars))
				}
				if len(cfg.Playout.Command) == 0 {
					t.Errorf("expected default playout command to survive, got empty")
				}
				ch := cfg.Playlists[0].Channels[0]
				if len(ch.Playout.TemplateVars) != 1 || ch.Playout.TemplateVars[0].Value != "6000" {
					t.Errorf("expected channel playout override, got %v", ch.Playout.TemplateVars)
				}
			},
		},
		{
			name: "legacy proxy.playout is rejected",
			configContent: `server:
  listen_addr: ":8080"
  public_url: "http://example.com"
url_generator:
  secret: "test-secret"
proxy:
  playout:
    command: [ffmpeg]`,
			expectError: true,
			validate:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "config-test")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			var configPath string
			if tt.configContent != "" {
				configDir := filepath.Join(tmpDir, "config")
				err = os.MkdirAll(configDir, 0755)
				if err != nil {
					t.Fatalf("failed to create config directory: %v", err)
				}

				configPath = configDir
				configFile := filepath.Join(configDir, "config.yaml")
				err = os.WriteFile(configFile, []byte(tt.configContent), 0644)
				if err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
			} else {
				configPath = filepath.Join(tmpDir, "nonexistent")
			}

			cfg, err := Load(configPath)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}
