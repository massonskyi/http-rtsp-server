package stream

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/protocol"
	"rstp-rsmt-server/internal/storage"
	"rstp-rsmt-server/internal/utils"
	"sync"
	"time"
)

// StreamManager управляет активными RTSP-потоками
type StreamManager struct {
	mutex   sync.RWMutex
	streams map[string]*Stream
	cfg     *config.Config
	logger  *utils.Logger
	storage *storage.Storage
	client  *protocol.RTSPClient
}

// Stream представляет один RTSP-поток
type Stream struct {
	ID         string
	StreamName string // Новое поле
	RTSPURL    string
	HLSPath    string
	StartedAt  time.Time
	Status     string
	cfg        *config.Config
	logger     *utils.Logger
	cancel     context.CancelFunc
	cmd        *exec.Cmd
}

// NewStreamManager создает новый StreamManager
func NewStreamManager(cfg *config.Config, logger *utils.Logger, storage *storage.Storage, client *protocol.RTSPClient) *StreamManager {
	return &StreamManager{
		streams: make(map[string]*Stream),
		cfg:     cfg,
		logger:  logger,
		storage: storage,
		client:  client,
	}
}

// StartStream запускает обработку RTSP-потока
func (sm *StreamManager) StartStream(rtspURL string, streamID string, streamName string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, exists := sm.streams[streamID]; exists {
		return fmt.Errorf("stream %s already exists", streamID)
	}

	// Создаем путь для HLS
	hlsDir := filepath.Join(sm.cfg.HLSDir, streamID)
	if err := utils.EnsureDir(hlsDir); err != nil {
		return fmt.Errorf("failed to create HLS directory: %w", err)
	}
	hlsPath := filepath.Join(hlsDir, "index.m3u8")

	// Создаем контекст для управления FFmpeg
	ctx, cancel := context.WithCancel(context.Background())

	// Создаем новый стрим
	stream := &Stream{
		ID:         streamID,
		StreamName: streamName,
		RTSPURL:    rtspURL,
		HLSPath:    hlsPath,
		StartedAt:  time.Now(),
		Status:     "running",
		cfg:        sm.cfg,
		logger:     sm.logger,
		cancel:     cancel,
	}

	// Сохраняем стрим
	sm.streams[streamID] = stream

	// Запускаем обработку RTSP-потока в горутине
	go func() {
		err := sm.client.ProcessStream(ctx, rtspURL, streamID, streamName, hlsPath)
		if err != nil {
			sm.mutex.Lock()
			if s, exists := sm.streams[streamID]; exists {
				s.Status = "failed"
			}
			sm.mutex.Unlock()
			sm.logger.Error("StartStream", "stream.go", fmt.Sprintf("Failed to process stream %s: %v", streamID, err))
		}
	}()

	return nil
}
func (sm *StreamManager) Storage() *storage.Storage {
	return sm.storage
}

// StopStream останавливает обработку RTSP-потока
func (sm *StreamManager) StopStream(streamID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	stream, exists := sm.streams[streamID]
	if !exists {
		return fmt.Errorf("stream %s not found", streamID)
	}

	// Отменяем контекст, чтобы завершить FFmpeg
	if stream.cancel != nil {
		stream.cancel()
	}

	// Обновляем статус
	stream.Status = "completed"

	// Сохраняем в архив
	archive := &database.Archive{
		StreamID:        streamID,
		StreamName:      stream.StreamName,
		Status:          stream.Status,
		Duration:        int(time.Since(stream.StartedAt).Seconds()),
		HLSPlaylistPath: stream.HLSPath,
		ArchivedAt:      time.Now(),
	}
	if err := sm.storage.ArchiveStream(context.Background(), archive); err != nil {
		sm.logger.Error("StopStream", "stream.go", fmt.Sprintf("Failed to save archive entry for stream %s: %v", streamID, err))
		return fmt.Errorf("failed to save archive entry: %w", err)
	}

	// Удаляем стрим из менеджера
	delete(sm.streams, streamID)

	return nil
}

// GetStream получает стрим по stream_id
func (sm *StreamManager) GetStream(streamID string) (*Stream, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stream, exists := sm.streams[streamID]
	return stream, exists
}

// GetStreamByName получает стрим по stream_name
func (sm *StreamManager) GetStreamByName(streamName string) (*Stream, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	for _, stream := range sm.streams {
		if stream.StreamName == streamName {
			return stream, true
		}
	}
	return nil, false
}

// ListStreams возвращает список всех активных стримов
func (sm *StreamManager) ListStreams() map[string]*Stream {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	streams := make(map[string]*Stream)
	for id, stream := range sm.streams {
		streams[id] = stream
	}
	return streams
}

// Shutdown останавливает все активные стримы
func (sm *StreamManager) Shutdown() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for streamID, stream := range sm.streams {
		if stream.cancel != nil {
			stream.cancel()
		}
		// Обновляем статус
		stream.Status = "completed"

		// Сохраняем в архив
		archive := &database.Archive{
			StreamID:        streamID,
			StreamName:      stream.StreamName,
			Status:          stream.Status,
			Duration:        int(time.Since(stream.StartedAt).Seconds()),
			HLSPlaylistPath: stream.HLSPath,
			ArchivedAt:      time.Now(),
		}
		if err := sm.storage.ArchiveStream(context.Background(), archive); err != nil {
			sm.logger.Error("Shutdown", "stream.go", fmt.Sprintf("Failed to save archive entry for stream %s: %v", streamID, err))
		}
	}
	sm.streams = make(map[string]*Stream)
}

// GetHLSPath возвращает путь к HLS-плейлисту
func (s *Stream) GetHLSPath() string {
	return s.HLSPath
}

// EnsureDir ensures that a directory exists, creating it if necessary.
func EnsureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, os.ModePerm)
	}
	return nil
}
