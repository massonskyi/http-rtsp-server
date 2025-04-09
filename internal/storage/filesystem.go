package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/utils"
)

// FileSystem предоставляет методы для работы с файловой системой
type FileSystem struct {
	cfg    *config.Config
	logger *utils.Logger
}

// NewFileSystem создает новый экземпляр FileSystem
func NewFileSystem(cfg *config.Config, logger *utils.Logger) *FileSystem {
	return &FileSystem{
		cfg:    cfg,
		logger: logger,
	}
}

// SaveVideoFile сохраняет видеофайл на диск
func (fs *FileSystem) SaveVideoFile(filename string, data io.Reader) (string, error) {
	// Формируем полный путь для сохранения файла
	filePath := filepath.Join(fs.cfg.VideoDir, filename)

	// Создаем файл
	file, err := os.Create(filePath)
	if err != nil {
		fs.logger.Errorf("SaveVideoFile", "filesystem.go", "Failed to create video file: %v", err)
		return "", fmt.Errorf("failed to create video file: %w", err)
	}
	defer file.Close()

	// Копируем данные в файл
	_, err = io.Copy(file, data)
	if err != nil {
		fs.logger.Errorf("SaveVideoFile", "filesystem.go", "Failed to write video file: %v", err)
		return "", fmt.Errorf("failed to write video file: %w", err)
	}

	fs.logger.Infof("SaveVideoFile", "filesystem.go", "Video file saved at: %s", filePath)
	return filePath, nil
}

// SaveThumbnailFile сохраняет миниатюру на диск
func (fs *FileSystem) SaveThumbnailFile(filename string, data io.Reader) (string, error) {
	// Формируем полный путь для сохранения файла
	filePath := filepath.Join(fs.cfg.ThumbnailDir, filename)

	// Создаем файл
	file, err := os.Create(filePath)
	if err != nil {
		fs.logger.Errorf("SaveThumbnailFile", "filesystem.go", "Failed to create thumbnail file: %v", err)
		return "", fmt.Errorf("failed to create thumbnail file: %w", err)
	}
	defer file.Close()

	// Копируем данные в файл
	_, err = io.Copy(file, data)
	if err != nil {
		fs.logger.Errorf("SaveThumbnailFile", "filesystem.go", "Failed to write thumbnail file: %v", err)
		return "", fmt.Errorf("failed to write thumbnail file: %w", err)
	}

	fs.logger.Infof("SaveThumbnailFile", "filesystem.go", "Thumbnail file saved at: %s", filePath)
	return filePath, nil
}
