package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"rstp-rsmt-server/internal/stream"
	"rstp-rsmt-server/internal/utils"
	"strconv"
	"strings"
	"time"
)

// StreamResponse представляет информацию о потоке для API
type StreamResponse struct {
	ID        string    `json:"id"`
	RTSPURL   string    `json:"rtsp_url"`
	HLSURL    string    `json:"hls_url"`
	HLSPath   string    `json:"hls_path"`
	Duration  int       `json:"duration"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"`
}

// Handler содержит зависимости для обработчиков
type Handler struct {
	logger        *utils.Logger
	streamManager *stream.StreamManager
	hlsManager    *stream.HLSManager
}

// NewHandler создает новый Handler
func NewHandler(logger *utils.Logger, streamManager *stream.StreamManager, hlsManager *stream.HLSManager) *Handler {
	return &Handler{
		logger:        logger,
		streamManager: streamManager,
		hlsManager:    hlsManager,
	}
}

// HealthHandler обрабатывает запросы к /health
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("HealthHandler", "handlers.go", "Health check endpoint called")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server is running"))
}

// StartStreamHandler обрабатывает запросы к /start-stream
func (h *Handler) StartStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rtspURL := r.FormValue("rtsp_url")
	if rtspURL == "" {
		http.Error(w, "Missing rtsp_url parameter", http.StatusBadRequest)
		return
	}

	streamID := r.FormValue("stream_id")
	if streamID == "" {
		streamID = fmt.Sprintf("stream_%d", time.Now().Unix())
	}
	h.logger.Info("StartStreamHandler", "handlers.go", fmt.Sprintf("Received request to start stream %s with URL %s", streamID, rtspURL))
	if err := h.streamManager.StartStream(rtspURL, streamID); err != nil {
		h.logger.Error("StartStreamHandler", "handlers.go", fmt.Sprintf("Failed to start stream %s: %v", streamID, err))
		http.Error(w, fmt.Sprintf("Failed to start stream: %v", err), http.StatusInternalServerError)
		return
	}

	// Даем немного времени на начало обработки
	time.Sleep(2 * time.Second)

	// Проверяем статус потока
	stream, exists := h.streamManager.GetStream(streamID)
	if !exists {
		h.logger.Error("StartStreamHandler", "handlers.go", fmt.Sprintf("Stream %s not found after starting", streamID))
		http.Error(w, "Stream not found after starting", http.StatusInternalServerError)
		return
	}
	if stream.Status == "failed" {
		h.logger.Error("StartStreamHandler", "handlers.go", fmt.Sprintf("Stream %s failed to start", streamID))
		http.Error(w, "Stream failed to start, check logs for details", http.StatusInternalServerError)
		return
	}

	h.logger.Info("StartStreamHandler", "handlers.go", fmt.Sprintf("Started processing stream: %s", rtspURL))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Stream processing started"))
}

// StopStreamHandler обрабатывает запросы к /stop-stream
func (h *Handler) StopStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	streamID := r.FormValue("stream_id")
	if streamID == "" {
		http.Error(w, "Missing stream_id parameter", http.StatusBadRequest)
		return
	}

	if err := h.streamManager.StopStream(streamID); err != nil {
		h.logger.Error("StopStreamHandler", "handlers.go", fmt.Sprintf("Failed to stop stream %s: %v", streamID, err))
		http.Error(w, fmt.Sprintf("Failed to stop stream: %v", err), http.StatusInternalServerError)
		return
	}

	h.logger.Info("StopStreamHandler", "handlers.go", fmt.Sprintf("Stopped stream: %s", streamID))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Stream stopped"))
}

// ListStreamsHandler обрабатывает запросы к /list-streams
func (h *Handler) ListStreamsHandler(w http.ResponseWriter, r *http.Request) {
	streams := h.streamManager.ListStreams()
	response := make(map[string]*StreamResponse)
	for id, stream := range streams {
		// Получаем метаданные из базы данных
		var duration int
		meta, err := h.streamManager.Storage().GetStreamMetadata(r.Context(), stream.ID)
		if err != nil {
			h.logger.Error("ListStreamsHandler", "handlers.go", fmt.Sprintf("Failed to get metadata for stream %s: %v", stream.ID, err))
			duration = 0 // Продолжаем с duration = 0
		} else {
			duration = meta.Duration
		}

		// Формируем HLS URL
		hlsURL := fmt.Sprintf("/stream/%s", stream.ID)

		response[id] = &StreamResponse{
			ID:        stream.ID,
			RTSPURL:   stream.RTSPURL,
			HLSURL:    hlsURL,
			HLSPath:   stream.GetHLSPath(),
			Duration:  duration,
			StartedAt: stream.StartedAt,
			Status:    stream.Status,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("ListStreamsHandler", "handlers.go", fmt.Sprintf("Failed to encode streams: %v", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// StreamHandler обрабатывает запросы к /stream/{stream_id} (только для активных стримов)
func (h *Handler) StreamHandler(w http.ResponseWriter, r *http.Request) {
	// Устанавливаем заголовки CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Обрабатываем предварительные запросы OPTIONS
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Извлекаем stream_id из URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		h.logger.Error("StreamHandler", "handlers.go", "Invalid URL format: too few path parts")
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	var streamID string
	var requestedPath string

	// Проверяем, есть ли параметр seek
	seekTimeStr := r.URL.Query().Get("time")
	var seekTime int
	if seekTimeStr != "" {
		var err error
		seekTime, err = strconv.Atoi(seekTimeStr)
		if err != nil || seekTime < 0 {
			h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Invalid seek time: %s", seekTimeStr))
			http.Error(w, "Invalid seek time", http.StatusBadRequest)
			return
		}
	}

	if len(pathParts) == 3 {
		// Возможны два случая:
		// 1. Запрос к плейлисту: /stream/stream1
		// 2. Запрос к сегменту с относительным путём: /stream/stream1_segment_002.ts
		possibleStreamIDOrSegment := pathParts[2]
		h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Processing request for: %s, seek time: %d", possibleStreamIDOrSegment, seekTime))

		// Проверяем, является ли это именем сегмента (например, stream1_segment_002.ts)
		if strings.Contains(possibleStreamIDOrSegment, "_segment_") && strings.HasSuffix(possibleStreamIDOrSegment, ".ts") {
			// Это сегмент, извлекаем streamID из имени сегмента
			parts := strings.Split(possibleStreamIDOrSegment, "_segment_")
			if len(parts) != 2 {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", possibleStreamIDOrSegment))
				http.Error(w, "Invalid segment name format", http.StatusBadRequest)
				return
			}
			streamID = parts[0] // Например, stream1
			segmentName := possibleStreamIDOrSegment

			// Проверяем, есть ли стрим в StreamManager
			stream, exists := h.streamManager.GetStream(streamID)
			if !exists {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Stream %s not found in StreamManager", streamID))
				http.Error(w, fmt.Sprintf("Stream %s is not active. Use /archive/%s to access archived streams", streamID, streamID), http.StatusNotFound)
				return
			}

			hlsPath := stream.GetHLSPath()
			if hlsPath == "" {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("HLS path for stream %s is empty", streamID))
				http.Error(w, "HLS playlist not available", http.StatusInternalServerError)
				return
			}
			requestedPath = filepath.Join(filepath.Dir(hlsPath), segmentName)
			h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving active segment: %s", requestedPath))
		} else {
			// Это запрос к плейлисту или seek (например, /stream/stream1 или /stream/stream1?time=60)
			streamID = possibleStreamIDOrSegment
			stream, exists := h.streamManager.GetStream(streamID)
			if !exists {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Stream %s not found in StreamManager", streamID))
				http.Error(w, fmt.Sprintf("Stream %s is not active. Use /archive/%s to access archived streams", streamID, streamID), http.StatusNotFound)
				return
			}

			hlsPath := stream.GetHLSPath()
			if hlsPath == "" {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("HLS path for stream %s is empty", streamID))
				http.Error(w, "HLS playlist not available", http.StatusInternalServerError)
				return
			}

			if seekTime > 0 {
				// Открываем оригинальный плейлист
				file, err := os.Open(hlsPath)
				if err != nil {
					h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Failed to open HLS playlist %s: %v", hlsPath, err))
					http.Error(w, "Failed to open HLS playlist", http.StatusInternalServerError)
					return
				}
				defer file.Close()

				// Вычисляем номер сегмента на основе времени
				segmentIndex := seekTime / 2 // Каждый сегмент 2 секунды (hls_time 2)
				segmentName := fmt.Sprintf("%s_segment_%03d.ts", streamID, segmentIndex)

				// Проверяем, существует ли сегмент
				segmentPath := filepath.Join(filepath.Dir(hlsPath), segmentName)
				if _, err := os.Stat(segmentPath); os.IsNotExist(err) {
					h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Segment not found for time %d: %s", seekTime, segmentPath))
					http.Error(w, fmt.Sprintf("Segment not found for time %d", seekTime), http.StatusNotFound)
					return
				}

				// Читаем оригинальный плейлист и создаём новый, начиная с нужного сегмента
				var newPlaylist strings.Builder
				scanner := bufio.NewScanner(file)
				var foundSegment bool
				var segmentDuration float64

				for scanner.Scan() {
					line := scanner.Text()
					// Копируем заголовки плейлиста
					if strings.HasPrefix(line, "#EXTM3U") || strings.HasPrefix(line, "#EXT-X-VERSION") || strings.HasPrefix(line, "#EXT-X-TARGETDURATION") || strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE") {
						newPlaylist.WriteString(line + "\n")
						continue
					}
					// Получаем длительность сегмента
					if strings.HasPrefix(line, "#EXTINF:") {
						durationStr := strings.TrimPrefix(line, "#EXTINF:")
						durationStr = strings.TrimSuffix(durationStr, ",")
						var err error
						segmentDuration, err = strconv.ParseFloat(durationStr, 64)
						if err != nil {
							h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Failed to parse segment duration: %v", err))
							segmentDuration = 2.0 // Предполагаем 2 секунды по умолчанию
						}
					}
					// Ищем нужный сегмент
					if strings.Contains(line, segmentName) {
						foundSegment = true
					}
					// Записываем сегменты, начиная с нужного
					if foundSegment {
						newPlaylist.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", segmentDuration))
						newPlaylist.WriteString(line + "\n")
					}
				}

				if err := scanner.Err(); err != nil {
					h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Error reading HLS playlist: %v", err))
					http.Error(w, "Error reading HLS playlist", http.StatusInternalServerError)
					return
				}

				if !foundSegment {
					h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Segment %s not found in playlist", segmentName))
					http.Error(w, fmt.Sprintf("Segment for time %d not found", seekTime), http.StatusNotFound)
					return
				}

				// Устанавливаем Content-Type и возвращаем новый плейлист
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving seek playlist starting at time %d", seekTime))
				w.Write([]byte(newPlaylist.String()))
				return
			}

			// Если seek не указан, возвращаем плейлист
			requestedPath = hlsPath
			h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving active playlist: %s", requestedPath))
		}
	} else if len(pathParts) == 4 {
		// Запрос к сегменту (например, /stream/stream1/stream1_segment_002.ts)
		streamID = pathParts[2]
		h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Processing segment request for streamID: %s", streamID))
		stream, exists := h.streamManager.GetStream(streamID)
		if !exists {
			h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Stream %s not found in StreamManager", streamID))
			http.Error(w, fmt.Sprintf("Stream %s is not active. Use /archive/%s to access archived streams", streamID, streamID), http.StatusNotFound)
			return
		}

		hlsPath := stream.GetHLSPath()
		if hlsPath == "" {
			h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("HLS path for stream %s is empty", streamID))
			http.Error(w, "HLS playlist not available", http.StatusInternalServerError)
			return
		}
		segmentName := pathParts[3]
		if !strings.HasPrefix(segmentName, streamID+"_segment_") || !strings.HasSuffix(segmentName, ".ts") {
			h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", segmentName))
			http.Error(w, "Invalid segment name format", http.StatusBadRequest)
			return
		}
		requestedPath = filepath.Join(filepath.Dir(hlsPath), segmentName)
		h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving active segment: %s", requestedPath))
	} else {
		h.logger.Error("StreamHandler", "handlers.go", "Invalid URL format: unexpected number of path parts")
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	// Проверяем, существует ли запрашиваемый файл
	if _, err := os.Stat(requestedPath); os.IsNotExist(err) {
		h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("File not found: %s", requestedPath))
		http.Error(w, fmt.Sprintf("File not found: %s", requestedPath), http.StatusNotFound)
		return
	}

	// Устанавливаем правильный Content-Type
	if strings.HasSuffix(requestedPath, ".m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	} else if strings.HasSuffix(requestedPath, ".ts") {
		w.Header().Set("Content-Type", "video/mp2t")
	}

	h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving file: %s", requestedPath))
	http.ServeFile(w, r, requestedPath)
}

// ListArchivedStreamsHandler обрабатывает запросы к /archive/list
func (h *Handler) ListArchivedStreamsHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем все записи из таблицы archive
	archives, err := h.streamManager.Storage().GetAllArchiveEntries(r.Context())
	if err != nil {
		h.logger.Error("ListArchivedStreamsHandler", "handlers.go", fmt.Sprintf("Failed to get archived streams: %v", err))
		http.Error(w, fmt.Sprintf("Failed to get archived streams: %v", err), http.StatusInternalServerError)
		return
	}

	response := make(map[string]*StreamResponse)
	for _, archive := range archives {
		// Получаем метаданные из stream_metadata
		var rtspURL string
		var startedAt time.Time
		meta, err := h.streamManager.Storage().GetStreamMetadata(r.Context(), archive.StreamID)
		if err != nil {
			h.logger.Error("ListArchivedStreamsHandler", "handlers.go", fmt.Sprintf("Failed to get metadata for stream %s: %v", archive.StreamID, err))
			// Если метаданные недоступны, используем пустые значения
			rtspURL = "unknown"
			startedAt = archive.ArchivedAt
		} else {
			// RTSP URL не хранится в базе, но можем использовать заглушку или логику для его восстановления
			rtspURL = "archived_stream"
			startedAt = meta.CreatedAt
		}

		hlsURL := fmt.Sprintf("/archive/%s", archive.StreamID)
		response[archive.StreamID] = &StreamResponse{
			ID:        archive.StreamID,
			RTSPURL:   rtspURL,
			HLSURL:    hlsURL,
			HLSPath:   archive.HLSPlaylistPath,
			Duration:  archive.Duration,
			StartedAt: startedAt,
			Status:    archive.Status,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("ListArchivedStreamsHandler", "handlers.go", fmt.Sprintf("Failed to encode archived streams: %v", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// ArchiveHandler обрабатывает запросы к /archive/{stream_id} (для архивных стримов)
func (h *Handler) ArchiveHandler(w http.ResponseWriter, r *http.Request) {
	// Устанавливаем заголовки CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Обрабатываем предварительные запросы OPTIONS
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Извлекаем stream_id из URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		h.logger.Error("ArchiveHandler", "handlers.go", "Invalid URL format: too few path parts")
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	var streamID string
	var requestedPath string

	// Проверяем, есть ли параметр seek
	seekTimeStr := r.URL.Query().Get("time")
	var seekTime int
	if seekTimeStr != "" {
		var err error
		seekTime, err = strconv.Atoi(seekTimeStr)
		if err != nil || seekTime < 0 {
			h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Invalid seek time: %s", seekTimeStr))
			http.Error(w, "Invalid seek time", http.StatusBadRequest)
			return
		}
	}

	if len(pathParts) == 3 {
		// Возможны два случая:
		// 1. Запрос к плейлисту: /archive/stream1
		// 2. Запрос к сегменту с относительным путём: /archive/stream1_segment_002.ts
		possibleStreamIDOrSegment := pathParts[2]
		h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Processing request for: %s, seek time: %d", possibleStreamIDOrSegment, seekTime))

		// Проверяем, является ли это именем сегмента (например, stream1_segment_002.ts)
		if strings.Contains(possibleStreamIDOrSegment, "_segment_") && strings.HasSuffix(possibleStreamIDOrSegment, ".ts") {
			// Это сегмент, извлекаем streamID из имени сегмента
			parts := strings.Split(possibleStreamIDOrSegment, "_segment_")
			if len(parts) != 2 {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", possibleStreamIDOrSegment))
				http.Error(w, "Invalid segment name format", http.StatusBadRequest)
				return
			}
			streamID = parts[0] // Например, stream1
			segmentName := possibleStreamIDOrSegment

			// Проверяем в архиве
			archiveEntry, err := h.streamManager.Storage().GetArchiveEntry(r.Context(), streamID)
			if err != nil {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Stream %s not found in archive: %v", streamID, err))
				http.Error(w, fmt.Sprintf("Stream %s not found in archive", streamID), http.StatusNotFound)
				return
			}
			hlsPath := archiveEntry.HLSPlaylistPath
			requestedPath = filepath.Join(filepath.Dir(hlsPath), segmentName)
			h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving archived segment: %s", requestedPath))
		} else {
			// Это запрос к плейлисту или seek (например, /archive/stream1 или /archive/stream1?time=60)
			streamID = possibleStreamIDOrSegment
			archiveEntry, err := h.streamManager.Storage().GetArchiveEntry(r.Context(), streamID)
			if err != nil {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Stream %s not found in archive: %v", streamID, err))
				http.Error(w, fmt.Sprintf("Stream %s not found in archive", streamID), http.StatusNotFound)
				return
			}

			hlsPath := archiveEntry.HLSPlaylistPath
			if seekTime > 0 {
				// Открываем оригинальный плейлист
				file, err := os.Open(hlsPath)
				if err != nil {
					h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Failed to open HLS playlist %s: %v", hlsPath, err))
					http.Error(w, "Failed to open HLS playlist", http.StatusInternalServerError)
					return
				}
				defer file.Close()

				// Вычисляем номер сегмента на основе времени
				segmentIndex := seekTime / 2 // Каждый сегмент 2 секунды (hls_time 2)
				segmentName := fmt.Sprintf("%s_segment_%03d.ts", streamID, segmentIndex)

				// Проверяем, существует ли сегмент
				segmentPath := filepath.Join(filepath.Dir(hlsPath), segmentName)
				if _, err := os.Stat(segmentPath); os.IsNotExist(err) {
					h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Segment not found for time %d: %s", seekTime, segmentPath))
					http.Error(w, fmt.Sprintf("Segment not found for time %d", seekTime), http.StatusNotFound)
					return
				}

				// Читаем оригинальный плейлист и создаём новый, начиная с нужного сегмента
				var newPlaylist strings.Builder
				scanner := bufio.NewScanner(file)
				var foundSegment bool
				var segmentDuration float64

				for scanner.Scan() {
					line := scanner.Text()
					// Копируем заголовки плейлиста
					if strings.HasPrefix(line, "#EXTM3U") || strings.HasPrefix(line, "#EXT-X-VERSION") || strings.HasPrefix(line, "#EXT-X-TARGETDURATION") || strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE") {
						newPlaylist.WriteString(line + "\n")
						continue
					}
					// Получаем длительность сегмента
					if strings.HasPrefix(line, "#EXTINF:") {
						durationStr := strings.TrimPrefix(line, "#EXTINF:")
						durationStr = strings.TrimSuffix(durationStr, ",")
						var err error
						segmentDuration, err = strconv.ParseFloat(durationStr, 64)
						if err != nil {
							h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Failed to parse segment duration: %v", err))
							segmentDuration = 2.0 // Предполагаем 2 секунды по умолчанию
						}
					}
					// Ищем нужный сегмент
					if strings.Contains(line, segmentName) {
						foundSegment = true
					}
					// Записываем сегменты, начиная с нужного
					if foundSegment {
						newPlaylist.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", segmentDuration))
						newPlaylist.WriteString(line + "\n")
					}
				}

				if err := scanner.Err(); err != nil {
					h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Error reading HLS playlist: %v", err))
					http.Error(w, "Error reading HLS playlist", http.StatusInternalServerError)
					return
				}

				if !foundSegment {
					h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Segment %s not found in playlist", segmentName))
					http.Error(w, fmt.Sprintf("Segment for time %d not found", seekTime), http.StatusNotFound)
					return
				}

				// Устанавливаем Content-Type и возвращаем новый плейлист
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving seek playlist starting at time %d", seekTime))
				w.Write([]byte(newPlaylist.String()))
				return
			}

			// Если seek не указан, возвращаем плейлист
			requestedPath = hlsPath
			h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving archived playlist: %s", requestedPath))
		}
	} else if len(pathParts) == 4 {
		// Запрос к сегменту (например, /archive/stream1/stream1_segment_002.ts)
		streamID = pathParts[2]
		h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Processing segment request for streamID: %s", streamID))
		archiveEntry, err := h.streamManager.Storage().GetArchiveEntry(r.Context(), streamID)
		if err != nil {
			h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Stream %s not found in archive: %v", streamID, err))
			http.Error(w, fmt.Sprintf("Stream %s not found in archive", streamID), http.StatusNotFound)
			return
		}
		hlsPath := archiveEntry.HLSPlaylistPath
		segmentName := pathParts[3]
		if !strings.HasPrefix(segmentName, streamID+"_segment_") || !strings.HasSuffix(segmentName, ".ts") {
			h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", segmentName))
			http.Error(w, "Invalid segment name format", http.StatusBadRequest)
			return
		}
		requestedPath = filepath.Join(filepath.Dir(hlsPath), segmentName)
		h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving archived segment: %s", requestedPath))
	} else {
		h.logger.Error("ArchiveHandler", "handlers.go", "Invalid URL format: unexpected number of path parts")
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	// Проверяем, существует ли запрашиваемый файл
	if _, err := os.Stat(requestedPath); os.IsNotExist(err) {
		h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("File not found: %s", requestedPath))
		http.Error(w, fmt.Sprintf("File not found: %s", requestedPath), http.StatusNotFound)
		return
	}

	// Устанавливаем правильный Content-Type
	if strings.HasSuffix(requestedPath, ".m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	} else if strings.HasSuffix(requestedPath, ".ts") {
		w.Header().Set("Content-Type", "video/mp2t")
	}

	h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving file: %s", requestedPath))
	http.ServeFile(w, r, requestedPath)
}
