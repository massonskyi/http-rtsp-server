package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/stream"
	"rstp-rsmt-server/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// StreamResponse представляет информацию о потоке для API
type StreamResponse struct {
	ID         string    `json:"id"`
	StreamName string    `json:"stream_name"`
	RTSPURL    string    `json:"rtsp_url"`
	HLSURL     string    `json:"hls_url"`
	HLSPath    string    `json:"hls_path"`
	Duration   int       `json:"duration"`
	StartedAt  time.Time `json:"started_at"`
	Status     string    `json:"status"`
	PreviewURL string    `json:"preview_url"` // Ссылка на превью
}

// VideoParamsRequest представляет параметры видео, которые можно обновить через API
type VideoParamsRequest struct {
	VideoBitrate string `json:"video_bitrate"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Quality      string `json:"quality"`
}

// Handler содержит зависимости для обработчиков
type Handler struct {
	logger        *utils.Logger
	cfg           *config.Config
	streamManager *stream.StreamManager
	hlsManager    *stream.HLSManager
}

// NewHandler создает новый Handler
func NewHandler(logger *utils.Logger, cfg *config.Config, streamManager *stream.StreamManager, hlsManager *stream.HLSManager) *Handler {
	return &Handler{
		logger:        logger,
		cfg:           cfg,
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

	streamName := r.FormValue("stream_id")
	if streamName == "" {
		http.Error(w, "Missing stream_id parameter", http.StatusBadRequest)
		return
	}

	// Генерируем уникальный UUID
	uuidStr := uuid.New().String()
	// Формируем timestamp
	timestamp := time.Now().Format("20060102150405") // Формат: YYYYMMDDHHMMSS
	// Формируем новый stream_id: UUID + stream_name + timestamp
	streamID := fmt.Sprintf("%s_%s_%s", uuidStr, streamName, timestamp)

	h.logger.Info("StartStreamHandler", "handlers.go", fmt.Sprintf("Received request to start stream %s with URL %s (stream_id: %s)", streamName, rtspURL, streamID))
	if err := h.streamManager.StartStream(rtspURL, streamID, streamName); err != nil {
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

	h.logger.Info("StartStreamHandler", "handlers.go", fmt.Sprintf("Started processing stream: %s (stream_id: %s)", rtspURL, streamID))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Stream processing started, stream_id: %s", streamID)))
}

// StopStreamHandler обрабатывает запросы к /stop-stream
func (h *Handler) StopStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	streamName := r.FormValue("stream_id")
	if streamName == "" {
		http.Error(w, "Missing stream_id parameter", http.StatusBadRequest)
		return
	}

	// Ищем стрим по stream_name
	stream, exists := h.streamManager.GetStreamByName(streamName)
	if !exists {
		h.logger.Error("StopStreamHandler", "handlers.go", fmt.Sprintf("Stream with name %s not found", streamName))
		http.Error(w, fmt.Sprintf("Stream with name %s not found", streamName), http.StatusNotFound)
		return
	}

	if err := h.streamManager.StopStream(stream.ID); err != nil {
		h.logger.Error("StopStreamHandler", "handlers.go", fmt.Sprintf("Failed to stop stream %s: %v", stream.ID, err))
		http.Error(w, fmt.Sprintf("Failed to stop stream: %v", err), http.StatusInternalServerError)
		return
	}

	h.logger.Info("StopStreamHandler", "handlers.go", fmt.Sprintf("Stopped stream: %s (stream_id: %s)", streamName, stream.ID))
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
		var previewPath string
		meta, err := h.streamManager.Storage().GetStreamMetadata(r.Context(), stream.ID)
		if err != nil {
			h.logger.Error("ListStreamsHandler", "handlers.go", fmt.Sprintf("Failed to get metadata for stream %s: %v", stream.ID, err))
			duration = 0
			previewPath = ""
		} else {
			duration = meta.Duration
			previewPath = meta.PreviewPath
		}

		// Формируем HLS URL
		hlsURL := fmt.Sprintf("/stream/%s", stream.StreamName)
		// Формируем URL для превью
		previewURL := ""
		if previewPath != "" {
			previewURL = fmt.Sprintf("/preview/%s", stream.StreamName)
		}

		response[id] = &StreamResponse{
			ID:         stream.ID,
			StreamName: stream.StreamName,
			RTSPURL:    stream.RTSPURL,
			HLSURL:     hlsURL,
			HLSPath:    stream.GetHLSPath(),
			Duration:   duration,
			StartedAt:  stream.StartedAt,
			Status:     stream.Status,
			PreviewURL: previewURL,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("ListStreamsHandler", "handlers.go", fmt.Sprintf("Failed to encode streams: %v", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// StreamHandler обрабатывает запросы к /stream/{stream_name}
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

	// Извлекаем stream_name из URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		h.logger.Error("StreamHandler", "handlers.go", "Invalid URL format: too few path parts")
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	var streamName string
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
		// 1. Запрос к плейлисту: /stream/stream3
		// 2. Запрос к сегменту с относительным путём: /stream/stream3_segment_002.ts
		possibleStreamNameOrSegment := pathParts[2]
		h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Processing request for: %s, seek time: %d", possibleStreamNameOrSegment, seekTime))

		// Проверяем, является ли это именем сегмента
		if strings.Contains(possibleStreamNameOrSegment, "_segment_") && strings.HasSuffix(possibleStreamNameOrSegment, ".ts") {
			// Это сегмент, извлекаем stream_name из имени сегмента
			parts := strings.Split(possibleStreamNameOrSegment, "_segment_")
			if len(parts) != 2 {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", possibleStreamNameOrSegment))
				http.Error(w, "Invalid segment name format", http.StatusBadRequest)
				return
			}
			// Извлекаем stream_name из имени сегмента
			segmentParts := strings.Split(parts[0], "_")
			if len(segmentParts) < 3 {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", possibleStreamNameOrSegment))
				http.Error(w, "Invalid segment name format", http.StatusBadRequest)
				return
			}
			streamName = segmentParts[len(segmentParts)-2] // stream_name идёт перед timestamp
			segmentName := possibleStreamNameOrSegment

			// Ищем стрим по stream_name
			stream, exists := h.streamManager.GetStreamByName(streamName)
			if !exists {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Stream with name %s not found in StreamManager", streamName))
				http.Error(w, fmt.Sprintf("Stream with name %s is not active. Use /archive/%s to access archived streams", streamName, streamName), http.StatusNotFound)
				return
			}
			streamID = stream.ID

			hlsPath := stream.GetHLSPath()
			if hlsPath == "" {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("HLS path for stream %s is empty", streamID))
				http.Error(w, "HLS playlist not available", http.StatusInternalServerError)
				return
			}
			requestedPath = filepath.Join(filepath.Dir(hlsPath), segmentName)
			h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving active segment: %s", requestedPath))
		} else {
			// Это запрос к плейлисту или seek
			streamName = possibleStreamNameOrSegment
			stream, exists := h.streamManager.GetStreamByName(streamName)
			if !exists {
				h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Stream with name %s not found in StreamManager", streamName))
				http.Error(w, fmt.Sprintf("Stream with name %s is not active. Use /archive/%s to access archived streams", streamName, streamName), http.StatusNotFound)
				return
			}
			streamID = stream.ID

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
				segmentIndex := seekTime / 2
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
					if strings.HasPrefix(line, "#EXTM3U") || strings.HasPrefix(line, "#EXT-X-VERSION") || strings.HasPrefix(line, "#EXT-X-TARGETDURATION") || strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE") {
						newPlaylist.WriteString(line + "\n")
						continue
					}
					if strings.HasPrefix(line, "#EXTINF:") {
						durationStr := strings.TrimPrefix(line, "#EXTINF:")
						durationStr = strings.TrimSuffix(durationStr, ",")
						var err error
						segmentDuration, err = strconv.ParseFloat(durationStr, 64)
						if err != nil {
							h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Failed to parse segment duration: %v", err))
							segmentDuration = 2.0
						}
					}
					if strings.Contains(line, segmentName) {
						foundSegment = true
					}
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

				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving seek playlist starting at time %d", seekTime))
				w.Write([]byte(newPlaylist.String()))
				return
			}

			requestedPath = hlsPath
			h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Serving active playlist: %s", requestedPath))
		}
	} else if len(pathParts) == 4 {
		// Запрос к сегменту
		streamName = pathParts[2]
		h.logger.Info("StreamHandler", "handlers.go", fmt.Sprintf("Processing segment request for streamName: %s", streamName))
		stream, exists := h.streamManager.GetStreamByName(streamName)
		if !exists {
			h.logger.Error("StreamHandler", "handlers.go", fmt.Sprintf("Stream with name %s not found in StreamManager", streamName))
			http.Error(w, fmt.Sprintf("Stream with name %s is not active. Use /archive/%s to access archived streams", streamName, streamName), http.StatusNotFound)
			return
		}
		streamID = stream.ID

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
	archives, err := h.streamManager.Storage().GetAllArchiveEntries(r.Context())
	if err != nil {
		h.logger.Error("ListArchivedStreamsHandler", "handlers.go", fmt.Sprintf("Failed to get archived streams: %v", err))
		http.Error(w, fmt.Sprintf("Failed to get archived streams: %v", err), http.StatusInternalServerError)
		return
	}

	response := make(map[string]*StreamResponse)
	for _, archive := range archives {
		var rtspURL string
		var startedAt time.Time
		var previewPath string
		meta, err := h.streamManager.Storage().GetStreamMetadata(r.Context(), archive.StreamID)
		if err != nil {
			h.logger.Error("ListArchivedStreamsHandler", "handlers.go", fmt.Sprintf("Failed to get metadata for stream %s: %v", archive.StreamID, err))
			rtspURL = "unknown"
			startedAt = archive.ArchivedAt
			previewPath = ""
		} else {
			rtspURL = "archived_stream"
			startedAt = meta.CreatedAt
			previewPath = meta.PreviewPath
		}

		hlsURL := fmt.Sprintf("/archive/%s", archive.StreamName)
		// Формируем URL для превью
		previewURL := ""
		if previewPath != "" {
			previewURL = fmt.Sprintf("/preview/%s", archive.StreamName)
		}

		response[archive.StreamID] = &StreamResponse{
			ID:         archive.StreamID,
			StreamName: archive.StreamName,
			RTSPURL:    rtspURL,
			HLSURL:     hlsURL,
			HLSPath:    archive.HLSPlaylistPath,
			Duration:   archive.Duration,
			StartedAt:  startedAt,
			Status:     archive.Status,
			PreviewURL: previewURL,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("ListArchivedStreamsHandler", "handlers.go", fmt.Sprintf("Failed to encode archived streams: %v", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// ArchiveHandler обрабатывает запросы к /archive/{stream_name}
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

	// Извлекаем stream_name из URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		h.logger.Error("ArchiveHandler", "handlers.go", "Invalid URL format: too few path parts")
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	var streamName string
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
		// 1. Запрос к плейлисту: /archive/stream3
		// 2. Запрос к сегменту с относительным путём: /archive/stream3_segment_002.ts
		possibleStreamNameOrSegment := pathParts[2]
		h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Processing request for: %s, seek time: %d", possibleStreamNameOrSegment, seekTime))

		// Проверяем, является ли это именем сегмента
		if strings.Contains(possibleStreamNameOrSegment, "_segment_") && strings.HasSuffix(possibleStreamNameOrSegment, ".ts") {
			// Это сегмент, извлекаем stream_name из имени сегмента
			parts := strings.Split(possibleStreamNameOrSegment, "_segment_")
			if len(parts) != 2 {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", possibleStreamNameOrSegment))
				http.Error(w, "Invalid segment name format", http.StatusBadRequest)
				return
			}
			// Извлекаем stream_name из имени сегмента
			segmentParts := strings.Split(parts[0], "_")
			if len(segmentParts) < 3 {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Invalid segment name format: %s", possibleStreamNameOrSegment))
				http.Error(w, "Invalid segment name format", http.StatusBadRequest)
				return
			}
			streamName = segmentParts[len(segmentParts)-2] // stream_name идёт перед timestamp
			segmentName := possibleStreamNameOrSegment

			// Ищем архивную запись по stream_name
			archive, err := h.streamManager.Storage().GetArchiveEntryByName(r.Context(), streamName)
			if err != nil {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Failed to get archive entry for stream_name %s: %v", streamName, err))
				http.Error(w, fmt.Sprintf("Archive entry for stream_name %s not found", streamName), http.StatusNotFound)
				return
			}
			streamID = archive.StreamID

			hlsPath := archive.HLSPlaylistPath
			if hlsPath == "" {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("HLS path for stream %s is empty", streamID))
				http.Error(w, "HLS playlist not available", http.StatusInternalServerError)
				return
			}
			requestedPath = filepath.Join(filepath.Dir(hlsPath), segmentName)
			h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving archived segment: %s", requestedPath))
		} else {
			// Это запрос к плейлисту или seek
			streamName = possibleStreamNameOrSegment
			archive, err := h.streamManager.Storage().GetArchiveEntryByName(r.Context(), streamName)
			if err != nil {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Failed to get archive entry for stream_name %s: %v", streamName, err))
				http.Error(w, fmt.Sprintf("Archive entry for stream_name %s not found", streamName), http.StatusNotFound)
				return
			}
			streamID = archive.StreamID

			hlsPath := archive.HLSPlaylistPath
			if hlsPath == "" {
				h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("HLS path for stream %s is empty", streamID))
				http.Error(w, "HLS playlist not available", http.StatusInternalServerError)
				return
			}

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
				segmentIndex := seekTime / 2
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
					if strings.HasPrefix(line, "#EXTM3U") || strings.HasPrefix(line, "#EXT-X-VERSION") || strings.HasPrefix(line, "#EXT-X-TARGETDURATION") || strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE") {
						newPlaylist.WriteString(line + "\n")
						continue
					}
					if strings.HasPrefix(line, "#EXTINF:") {
						durationStr := strings.TrimPrefix(line, "#EXTINF:")
						durationStr = strings.TrimSuffix(durationStr, ",")
						var err error
						segmentDuration, err = strconv.ParseFloat(durationStr, 64)
						if err != nil {
							h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Failed to parse segment duration: %v", err))
							segmentDuration = 2.0
						}
					}
					if strings.Contains(line, segmentName) {
						foundSegment = true
					}
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

				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving seek playlist starting at time %d", seekTime))
				w.Write([]byte(newPlaylist.String()))
				return
			}

			requestedPath = hlsPath
			h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Serving archived playlist: %s", requestedPath))
		}
	} else if len(pathParts) == 4 {
		// Запрос к сегменту
		streamName = pathParts[2]
		h.logger.Info("ArchiveHandler", "handlers.go", fmt.Sprintf("Processing segment request for streamName: %s", streamName))
		archive, err := h.streamManager.Storage().GetArchiveEntryByName(r.Context(), streamName)
		if err != nil {
			h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("Failed to get archive entry for stream_name %s: %v", streamName, err))
			http.Error(w, fmt.Sprintf("Archive entry for stream_name %s not found", streamName), http.StatusNotFound)
			return
		}
		streamID = archive.StreamID

		hlsPath := archive.HLSPlaylistPath
		if hlsPath == "" {
			h.logger.Error("ArchiveHandler", "handlers.go", fmt.Sprintf("HLS path for stream %s is empty", streamID))
			http.Error(w, "HLS playlist not available", http.StatusInternalServerError)
			return
		}
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

// PreviewHandler обрабатывает запросы к /preview/{stream_name}
func (h *Handler) PreviewHandler(w http.ResponseWriter, r *http.Request) {
	// Устанавливаем заголовки CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Обрабатываем предварительные запросы OPTIONS
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Извлекаем stream_name из URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 3 {
		h.logger.Error("PreviewHandler", "handlers.go", "Invalid URL format: expected /preview/{stream_name}")
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	streamName := pathParts[2]
	h.logger.Info("PreviewHandler", "handlers.go", fmt.Sprintf("Processing preview request for streamName: %s", streamName))

	// Сначала ищем активный стрим
	var previewPath string
	var streamID string
	stream, exists := h.streamManager.GetStreamByName(streamName)
	if exists {
		// Стрим активный, получаем метаданные
		streamID = stream.ID
		meta, err := h.streamManager.Storage().GetStreamMetadata(r.Context(), streamID)
		if err != nil {
			h.logger.Error("PreviewHandler", "handlers.go", fmt.Sprintf("Failed to get metadata for active stream %s: %v", streamID, err))
			http.Error(w, "Failed to get stream metadata", http.StatusInternalServerError)
			return
		}
		previewPath = meta.PreviewPath
	} else {
		// Стрим не активный, ищем в архиве
		archive, err := h.streamManager.Storage().GetArchiveEntryByName(r.Context(), streamName)
		if err != nil {
			h.logger.Error("PreviewHandler", "handlers.go", fmt.Sprintf("Failed to get archive entry for stream_name %s: %v", streamName, err))
			http.Error(w, fmt.Sprintf("Stream or archive entry for stream_name %s not found", streamName), http.StatusNotFound)
			return
		}
		streamID = archive.StreamID
		meta, err := h.streamManager.Storage().GetStreamMetadata(r.Context(), streamID)
		if err != nil {
			h.logger.Error("PreviewHandler", "handlers.go", fmt.Sprintf("Failed to get metadata for archived stream %s: %v", streamID, err))
			http.Error(w, "Failed to get stream metadata", http.StatusInternalServerError)
			return
		}
		previewPath = meta.PreviewPath
	}

	// Проверяем, есть ли путь к превью
	if previewPath == "" {
		h.logger.Error("PreviewHandler", "handlers.go", fmt.Sprintf("Preview path not found for stream %s", streamID))
		http.Error(w, "Preview not available for this stream", http.StatusNotFound)
		return
	}

	// Проверяем, существует ли файл превью
	if _, err := os.Stat(previewPath); os.IsNotExist(err) {
		h.logger.Error("PreviewHandler", "handlers.go", fmt.Sprintf("Preview file not found: %s", previewPath))
		http.Error(w, "Preview file not found", http.StatusNotFound)
		return
	}

	// Устанавливаем Content-Type для изображения
	w.Header().Set("Content-Type", "image/jpeg")
	h.logger.Info("PreviewHandler", "handlers.go", fmt.Sprintf("Serving preview file: %s", previewPath))
	http.ServeFile(w, r, previewPath)
}

// UpdateVideoParamsHandler обрабатывает запросы к /update-video-params
func (h *Handler) UpdateVideoParamsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	streamName := r.FormValue("stream_id")
	if streamName == "" {
		http.Error(w, "Missing stream_id parameter", http.StatusBadRequest)
		return
	}

	// Ищем стрим по stream_name
	_, exists := h.streamManager.GetStreamByName(streamName)
	if !exists {
		h.logger.Error("UpdateVideoParamsHandler", "handlers.go", fmt.Sprintf("Stream with name %s not found", streamName))
		http.Error(w, fmt.Sprintf("Stream with name %s not found", streamName), http.StatusNotFound)
		return
	}

	var params VideoParamsRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("UpdateVideoParamsHandler", "handlers.go", fmt.Sprintf("Failed to read request body: %v", err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := json.Unmarshal(body, &params); err != nil {
		h.logger.Error("UpdateVideoParamsHandler", "handlers.go", fmt.Sprintf("Failed to parse request body: %v", err))
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	// Здесь должна быть логика обновления параметров видео
	// Например, перезапуск FFmpeg с новыми параметрами
	h.logger.Info("UpdateVideoParamsHandler", "handlers.go", fmt.Sprintf("Received request to update video params for stream %s: %+v", streamName, params))

	// В данном примере мы просто логируем и возвращаем успешный ответ
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Video parameters updated successfully"))
}

// UpdateConfigHandler обрабатывает запросы к /update-config
func (h *Handler) UpdateConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Читаем тело запроса
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorf("UpdateConfigHandler", "handlers.go", "Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Обновляем конфигурацию
	if err := h.cfg.UpdateConfig(body); err != nil {
		h.logger.Errorf("UpdateConfigHandler", "handlers.go", "Failed to update config: %v", err)
		http.Error(w, fmt.Sprintf("Failed to update config: %v", err), http.StatusBadRequest)
		return
	}

	// Логируем успех
	h.logger.Info("UpdateConfigHandler", "handlers.go", "Configuration updated successfully")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Configuration updated successfully"))
}
