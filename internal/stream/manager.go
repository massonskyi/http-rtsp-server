package stream

import (
	"context"
	"fmt"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/protocol"
	"rstp-rsmt-server/internal/storage"
	"rstp-rsmt-server/internal/utils"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
)

// StreamManager управляет активными RTSP-потоками
type StreamManager struct {
	cfg        *config.Config
	logger     *utils.Logger
	storage    *storage.Storage
	fs         *storage.FileSystem
	rtspClient *protocol.RTSPClient
	streams    map[string]*Stream
	streamsMu  sync.RWMutex
	pool       *ants.Pool
}

// Stream представляет активный RTSP-поток
type Stream struct {
	ID        string
	RTSPURL   string
	Cancel    context.CancelFunc
	StartedAt time.Time
	Status    string
	Done      chan struct{}
}

// StreamInfo содержит информацию о потоке
type StreamInfo struct {
	RTSPURL   string
	StartedAt time.Time
	Cancel    context.CancelFunc
}

// NewStreamManager создает новый StreamManager
func NewStreamManager(cfg *config.Config, logger *utils.Logger, storage *storage.Storage, fs *storage.FileSystem, rtspClient *protocol.RTSPClient) (*StreamManager, error) {
	pool, err := ants.NewPool(100, ants.WithPanicHandler(func(err interface{}) {
		logger.Errorf("StreamManager", "manager.go", "Panic in pool: %v", err)
	}))
	if err != nil {
		return nil, err
	}

	return &StreamManager{
		cfg:        cfg,
		logger:     logger,
		storage:    storage,
		fs:         fs,
		rtspClient: rtspClient,
		streams:    make(map[string]*Stream),
		pool:       pool,
	}, nil
}

// GetConfig возвращает конфигурацию
func (m *StreamManager) GetConfig() *config.Config {
	return m.cfg
}
func (m *StreamManager) GetStorage() *storage.Storage {
	return m.storage
}

// StartStream запускает обработку RTSP-потока
func (m *StreamManager) StartStream(rtspURL, streamID string) error {
	m.streamsMu.Lock()
	defer m.streamsMu.Unlock()

	if _, exists := m.streams[streamID]; exists {
		m.logger.Errorf("StartStream", "manager.go", "Stream %s already exists", streamID)
		return fmt.Errorf("stream %s already exists", streamID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	doneChan := make(chan struct{})

	m.streams[streamID] = &Stream{
		ID:        streamID,
		RTSPURL:   rtspURL,
		Cancel:    cancel,
		StartedAt: time.Now(),
		Status:    "running",
		Done:      doneChan,
	}

	err := m.pool.Submit(func() {
		defer func() {
			m.streamsMu.Lock()
			if stream, exists := m.streams[streamID]; exists {
				stream.Status = "stopped"
				close(stream.Done)
			}
			m.streamsMu.Unlock()
		}()

		if err := m.rtspClient.ProcessStream(ctx, rtspURL, streamID); err != nil {
			// Проверяем, была ли ошибка вызвана отменой контекста
			if ctx.Err() != nil {
				m.logger.Infof("StartStream", "manager.go", "Stream %s was canceled: %v", streamID, err)
				return
			}
			m.logger.Errorf("StartStream", "manager.go", "Failed to process stream %s: %v", streamID, err)
		}
	})
	if err != nil {
		delete(m.streams, streamID)
		cancel()
		close(doneChan)
		m.logger.Errorf("StartStream", "manager.go", "Failed to submit stream %s to pool: %v", streamID, err)
		return fmt.Errorf("failed to submit stream %s to pool: %v", streamID, err)
	}

	m.logger.Infof("StartStream", "manager.go", "Started stream %s with URL %s", streamID, rtspURL)
	return nil
}

// StopStream останавливает обработку RTSP-потока
func (m *StreamManager) StopStream(streamID string) error {
	m.streamsMu.Lock()
	stream, exists := m.streams[streamID]
	if !exists {
		m.streamsMu.Unlock()
		m.logger.Errorf("StopStream", "manager.go", "Stream %s not found", streamID)
		return fmt.Errorf("stream %s not found", streamID)
	}

	// Вызываем Cancel для остановки обработки
	stream.Cancel()
	stream.Status = "stopping"
	doneChan := stream.Done
	m.streamsMu.Unlock()

	// Ожидаем завершения обработки
	select {
	case <-doneChan:
		m.logger.Infof("StopStream", "manager.go", "Stream %s processing completed", streamID)
	case <-time.After(10 * time.Second):
		m.logger.Warningf("StopStream", "manager.go", "Stream %s did not stop within 10 seconds, forcing stop", streamID)
	}

	// Удаляем поток из списка
	m.streamsMu.Lock()
	delete(m.streams, streamID)
	m.streamsMu.Unlock()

	m.logger.Infof("StopStream", "manager.go", "Stopped stream %s", streamID)
	return nil
}

// GetStream возвращает информацию о потоке
func (m *StreamManager) GetStream(streamID string) (*Stream, bool) {
	m.streamsMu.RLock()
	defer m.streamsMu.RUnlock()

	stream, exists := m.streams[streamID]
	return stream, exists
}

// ListStreams возвращает список активных потоков
func (m *StreamManager) ListStreams() []*Stream {
	m.streamsMu.RLock()
	defer m.streamsMu.RUnlock()

	streams := make([]*Stream, 0, len(m.streams))
	for _, stream := range m.streams {
		streams = append(streams, stream)
	}
	return streams
}

// Shutdown останавливает все активные потоки
func (m *StreamManager) Shutdown() {
	m.streamsMu.Lock()
	defer m.streamsMu.Unlock()

	for _, stream := range m.streams {
		stream.Cancel()
		stream.Status = "stopping"
	}
	m.pool.Release()
	m.logger.Info("Shutdown", "manager.go", "StreamManager shut down")
}
