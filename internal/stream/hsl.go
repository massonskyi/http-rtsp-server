package stream

import (
	"os"
	"os/exec"
	"path/filepath"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/utils"
)

// HLSManager управляет генерацией HLS-плейлистов и сегментов
type HLSManager struct {
	cfg    *config.Config
	logger *utils.Logger
}

// NewHLSManager создает новый HLSManager
func NewHLSManager(cfg *config.Config, logger *utils.Logger) *HLSManager {
	return &HLSManager{
		cfg:    cfg,
		logger: logger,
	}
}

// GenerateHLS генерирует HLS-плейлист и сегменты для видео
func (m *HLSManager) GenerateHLS(videoPath, streamID string) (string, error) {
	// Создаем директорию для HLS-сегментов
	hlsDir := filepath.Join(m.cfg.HLSDir, streamID)
	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		m.logger.Errorf("GenerateHLS", "hls.go", "Failed to create HLS directory: %v", err)
		return "", err
	}

	// Формируем пути для плейлиста и сегментов
	playlistPath := filepath.Join(hlsDir, "playlist.m3u8")
	segmentPattern := filepath.Join(hlsDir, "segment%03d.ts")

	// Используем FFmpeg для генерации HLS
	ffmpegCmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-hls_time", "10", // Длительность сегмента 10 секунд
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	)
	if err := ffmpegCmd.Run(); err != nil {
		m.logger.Errorf("GenerateHLS", "hls.go", "Failed to generate HLS: %v", err)
		return "", err
	}

	m.logger.Infof("GenerateHLS", "hls.go", "Generated HLS playlist for stream %s at %s", streamID, playlistPath)
	return playlistPath, nil
}
