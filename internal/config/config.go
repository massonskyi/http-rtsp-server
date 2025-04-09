package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	DatabaseURL  string
	VideoDir     string
	ThumbnailDir string
	ServerPort   int
	ReservedPort int
	HLSDir       string
}

// LoadConfig loads and validates the application configuration
func LoadConfig() (*Config, error) {
	// Parse ports with validation
	var err error

	// Load .env file
	err = godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	cfg := &Config{
		DatabaseURL:  getRequiredEnv("DATABASE_URL"),
		VideoDir:     getRequiredEnv("VIDEO_DIR"),
		ThumbnailDir: getRequiredEnv("THUMBNAIL_DIR"),
		HLSDir:       getRequiredEnv("HLS_DIR"),
	}

	cfg.ServerPort, err = getPortEnv("SERVER_PORT")
	if err != nil {
		return nil, fmt.Errorf("invalid server port: %w", err)
	}

	cfg.ReservedPort, err = getPortEnv("RESERVED_PORT")
	if err != nil {
		return nil, fmt.Errorf("invalid reserved port: %w", err)
	}

	// Ensure directories exist with proper permissions
	if err := ensureDirectory(cfg.VideoDir); err != nil {
		return nil, fmt.Errorf("video directory error: %w", err)
	}

	if err := ensureDirectory(cfg.ThumbnailDir); err != nil {
		return nil, fmt.Errorf("thumbnail directory error: %w", err)
	}

	if err := ensureDirectory(cfg.HLSDir); err != nil {
		return nil, fmt.Errorf("HSL directory error: %w", err)
	}
	return cfg, nil
}

// getRequiredEnv retrieves a required environment variable
func getRequiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("required environment variable %s is missing", key))
	}
	return value
}

// getPortEnv retrieves and validates a port number from environment
func getPortEnv(key string) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return 0, fmt.Errorf("port not specified")
	}

	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid port number: %w", err)
	}

	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %d out of range (1-65535)", port)
	}

	return port, nil
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
