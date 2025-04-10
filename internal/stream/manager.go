package stream

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/protocol"
	"rstp-rsmt-server/internal/storage"
	"rstp-rsmt-server/internal/utils"
	"sync"
	"time"
)

// Stream представляет RTSP-поток
type Stream struct {
	ID        string
	RTSPURL   string
	HLSPath   string
	StartedAt time.Time
	Status    string
	logger    *utils.Logger
	cfg       *config.Config
	storage   *storage.Storage
}

// StreamManager управляет потоками
type StreamManager struct {
	streams    map[string]*Stream
	mutex      sync.RWMutex
	logger     *utils.Logger
	cfg        *config.Config
	storage    *storage.Storage
	rtspClient *protocol.RTSPClient
}

// NewStreamManager создает новый StreamManager
func NewStreamManager(cfg *config.Config, logger *utils.Logger, storage *storage.Storage, rtspClient *protocol.RTSPClient) *StreamManager {
	return &StreamManager{
		streams:    make(map[string]*Stream),
		logger:     logger,
		cfg:        cfg,
		storage:    storage,
		rtspClient: rtspClient,
	}
}

// Shutdown останавливает все активные стримы
func (sm *StreamManager) Shutdown() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for streamID := range sm.streams {
		if err := sm.StopStream(streamID); err != nil {
			sm.logger.Error("Shutdown", "stream.go", fmt.Sprintf("Failed to stop stream %s: %v", streamID, err))
		}
	}
}

// StartStream запускает обработку RTSP-потока
func (sm *StreamManager) StartStream(rtspURL, streamID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, exists := sm.streams[streamID]; exists {
		return fmt.Errorf("stream %s already exists", streamID)
	}

	// Создаем директорию для HLS
	hlsDir := filepath.Join(sm.cfg.HLSDir, streamID)
	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		sm.logger.Error("StartStream", "stream.go", fmt.Sprintf("Failed to create HLS directory for stream %s: %v", streamID, err))
		return fmt.Errorf("failed to create HLS directory: %w", err)
	}

	// Путь к HLS-плейлисту
	hlsPlaylist := filepath.Join(hlsDir, fmt.Sprintf("%s.m3u8", streamID))

	// Создаем стрим
	stream := &Stream{
		ID:        streamID,
		RTSPURL:   rtspURL,
		HLSPath:   hlsPlaylist,
		StartedAt: time.Now(),
		Status:    "processing",
		logger:    sm.logger,
		cfg:       sm.cfg,
		storage:   sm.storage,
	}

	// Сохраняем стрим в менеджере
	sm.streams[streamID] = stream

	// Запускаем обработку RTSP-потока через RTSPClient в отдельной горутине
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := sm.rtspClient.ProcessStream(ctx, rtspURL, streamID, hlsPlaylist); err != nil {
			sm.logger.Error("StartStream", "stream.go", fmt.Sprintf("Failed to process stream %s: %v", streamID, err))
			// Обновляем статус стрима
			stream.Status = "failed"
			return
		}

		// Если обработка завершена успешно, обновляем статус
		stream.Status = "completed"
	}()

	// Запускаем горутину для обновления продолжительности
	go stream.updateDuration()

	return nil
}

// StopStream останавливает обработку RTSP-потока
func (sm *StreamManager) StopStream(streamID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	stream, exists := sm.streams[streamID]
	if !exists {
		return fmt.Errorf("stream %s not found", streamID)
	}

	// Обновляем статус
	stream.Status = "completed"

	// Сохраняем в архив
	archive := &database.Archive{
		StreamID:        streamID,
		Status:          stream.Status,
		Duration:        stream.getDuration(),
		HLSPlaylistPath: stream.HLSPath,
		ArchivedAt:      time.Now(),
	}
	if err := sm.storage.SaveArchiveEntry(context.Background(), archive); err != nil {
		sm.logger.Error("StopStream", "stream.go", fmt.Sprintf("Failed to save archive entry for stream %s: %v", streamID, err))
		// Не прерываем выполнение, но логируем ошибку
	}

	// Удаляем стрим из менеджера
	delete(sm.streams, streamID)

	return nil
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

// GetStream возвращает стрим по ID
func (sm *StreamManager) GetStream(streamID string) (*Stream, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stream, exists := sm.streams[streamID]
	return stream, exists
}

// Storage возвращает хранилище
func (sm *StreamManager) Storage() *storage.Storage {
	return sm.storage
}

// updateDuration обновляет продолжительность стрима
func (s *Stream) updateDuration() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.Status != "processing" {
				return
			}
			duration := s.getDuration()
			if err := s.storage.UpdateStreamMetadataStatus(context.Background(), s.ID, duration); err != nil {
				s.logger.Error("updateDuration", "stream.go", fmt.Sprintf("Failed to update duration for stream %s: %v", s.ID, err))
			}
		}
	}
}

// getDuration возвращает текущую продолжительность стрима
func (s *Stream) getDuration() int {
	return int(time.Since(s.StartedAt).Seconds())
}

// GetHLSPath возвращает путь к HLS-плейлисту
func (s *Stream) GetHLSPath() string {
	return s.HLSPath
}
