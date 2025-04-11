package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config holds all application configuration
type Config struct {
	mu           sync.RWMutex
	DatabaseURL  string       `json:"database_url"`
	VideoDir     string       `json:"video_dir"`
	ThumbnailDir string       `json:"thumbnail_dir"`
	ServerPort   int          `json:"server_port"`
	ReservedPort int          `json:"reserved_port"`
	HLSDir       string       `json:"hls_dir"`
	FFmpeg       FFmpegParams `json:"ffmpeg"`
}

// FFmpegParams contains FFmpeg configuration parameters
type FFmpegParams struct {
	VideoBitrate    string `json:"video_bitrate"`
	VideoMaxRate    string `json:"video_max_rate"`
	VideoMinRate    string `json:"video_min_rate"`
	VideoBufSize    string `json:"video_buf_size"`
	FrameRate       string `json:"frame_rate"`
	GOPSize         int    `json:"gop_size"`
	KeyIntMin       int    `json:"key_int_min"`
	HLSListSize     string `json:"hls_list_size"`
	HLSSegmentTime  string `json:"hls_segment_time"`
	AudioBitrate    string `json:"audio_bitrate"`
	AudioSampleRate string `json:"audio_sample_rate"`
}

// LoadConfig loads and validates the application configuration from config.json
func LoadConfig() (*Config, error) {
	// Default configuration
	cfg := &Config{
		DatabaseURL:  "postgres://user:password@localhost:5432/dbname?sslmode=disable",
		VideoDir:     "videos",
		ThumbnailDir: "thumbnails",
		HLSDir:       "hls",
		ServerPort:   8080,
		ReservedPort: 8081,
		FFmpeg: FFmpegParams{
			VideoBitrate:    "2000k",
			VideoMaxRate:    "2500k",
			VideoMinRate:    "1500k",
			VideoBufSize:    "3000k",
			FrameRate:       "30",
			GOPSize:         30,
			KeyIntMin:       30,
			HLSListSize:     "0",
			HLSSegmentTime:  "2",
			AudioBitrate:    "128k",
			AudioSampleRate: "44100",
		},
	}

	// Read config file
	data, err := os.ReadFile("config.json")
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, use defaults
			return validateAndEnsureDirs(cfg)
		}
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Parse JSON
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("error parsing config JSON: %w", err)
	}

	return validateAndEnsureDirs(cfg)
}

// UpdateConfig updates the configuration with new values from a JSON byte slice
func (cfg *Config) UpdateConfig(newConfigData []byte) error {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	var newCfg Config
	if err := json.Unmarshal(newConfigData, &newCfg); err != nil {
		return fmt.Errorf("error parsing new config JSON: %w", err)
	}

	// Update fields
	cfg.DatabaseURL = newCfg.DatabaseURL
	cfg.VideoDir = newCfg.VideoDir
	cfg.ThumbnailDir = newCfg.ThumbnailDir
	cfg.ServerPort = newCfg.ServerPort
	cfg.ReservedPort = newCfg.ReservedPort
	cfg.HLSDir = newCfg.HLSDir
	cfg.FFmpeg = newCfg.FFmpeg

	// Сохраняем обновлённую конфигурацию в файл
	updatedData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling updated config: %w", err)
	}
	if err := os.WriteFile("config.json", updatedData, 0644); err != nil {
		return fmt.Errorf("error writing updated config to file: %w", err)
	}

	// Validate and ensure directories
	_, err = validateAndEnsureDirs(cfg)
	return err
}

// GetFFmpeg safely retrieves the FFmpeg configuration
func (cfg *Config) GetFFmpeg() FFmpegParams {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	return cfg.FFmpeg
}

// GetServerPort safely retrieves the ServerPort
func (cfg *Config) GetServerPort() int {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	return cfg.ServerPort
}

// validateAndEnsureDirs validates the configuration and ensures directories exist
func validateAndEnsureDirs(cfg *Config) (*Config, error) {
	// Validate ports
	if cfg.ServerPort < 1 || cfg.ServerPort > 65535 {
		return nil, fmt.Errorf("server port %d out of range (1-65535)", cfg.ServerPort)
	}
	if cfg.ReservedPort < 1 || cfg.ReservedPort > 65535 {
		return nil, fmt.Errorf("reserved port %d out of range (1-65535)", cfg.ReservedPort)
	}

	// Validate required fields
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database_url is required")
	}
	if cfg.VideoDir == "" {
		return nil, fmt.Errorf("video_dir is required")
	}
	if cfg.ThumbnailDir == "" {
		return nil, fmt.Errorf("thumbnail_dir is required")
	}
	if cfg.HLSDir == "" {
		return nil, fmt.Errorf("hls_dir is required")
	}

	// Ensure directories exist with proper permissions
	if err := ensureDirectory(cfg.VideoDir); err != nil {
		return nil, fmt.Errorf("video directory error: %w", err)
	}
	if err := ensureDirectory(cfg.ThumbnailDir); err != nil {
		return nil, fmt.Errorf("thumbnail directory error: %w", err)
	}
	if err := ensureDirectory(cfg.HLSDir); err != nil {
		return nil, fmt.Errorf("HLS directory error: %w", err)
	}

	return cfg, nil
}

// ensureDirectory creates a directory if it doesn't exist with secure permissions
func ensureDirectory(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// 0755 - owner can read/write/execute, group/others can read/execute
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Verify the directory is actually accessible
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("directory access verification failed: %w", err)
	}

	return nil
}
