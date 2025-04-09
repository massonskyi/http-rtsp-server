package api

import (
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

// VideoResponse представляет информацию о видео для API
type VideoResponse struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	FilePath     string    `json:"file_path"`
	HLSURL       string    `json:"hls_url"` // URL для доступа к HLS-плейлисту
	ThumbnailURL string    `json:"thumbnail_url"`
	Duration     int       `json:"duration"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
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
	h.logger.Info("healthHandler", "handlers.go", "Health check endpoint called")
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

	if err := h.streamManager.StartStream(rtspURL, streamID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start stream: %v", err), http.StatusInternalServerError)
		return
	}

	h.logger.Info("startStreamHandler", "handlers.go", fmt.Sprintf("Started processing stream: %s", rtspURL))
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
		http.Error(w, fmt.Sprintf("Failed to stop stream: %v", err), http.StatusInternalServerError)
		return
	}

	h.logger.Info("stopStreamHandler", "handlers.go", fmt.Sprintf("Stopped stream: %s", streamID))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Stream stopped"))
}

// ListStreamsHandler обрабатывает запросы к /list-streams
func (h *Handler) ListStreamsHandler(w http.ResponseWriter, r *http.Request) {
	streams := h.streamManager.ListStreams()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(streams); err != nil {
		h.logger.Error("listStreamsHandler", "handlers.go", fmt.Sprintf("Failed to encode streams: %v", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// StreamHandler обрабатывает запросы к /stream/{stream_id}
func (h *Handler) StreamHandler(w http.ResponseWriter, r *http.Request) {
	// Извлекаем stream_id из URL
	streamID := filepath.Base(r.URL.Path)
	if streamID == "" {
		http.Error(w, "Missing stream_id in URL", http.StatusBadRequest)
		return
	}

	// Проверяем, существует ли поток
	stream, exists := h.streamManager.GetStream(streamID)
	if !exists {
		http.Error(w, "Stream not found", http.StatusNotFound)
		return
	}

	// Проверяем, существует ли видео
	videoPath := filepath.Join(h.streamManager.GetConfig().VideoDir, fmt.Sprintf("%s_%d.mp4", streamID, stream.StartedAt.Unix()))
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		http.Error(w, "Video file not found", http.StatusNotFound)
		return
	}

	// Генерируем HLS-плейлист
	playlistPath, err := h.hlsManager.GenerateHLS(videoPath, streamID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate HLS: %v", err), http.StatusInternalServerError)
		return
	}

	// Отдаем плейлист
	http.ServeFile(w, r, playlistPath)
}

// ListVideosHandler обрабатывает запросы к /videos
func (h *Handler) ListVideosHandler(w http.ResponseWriter, r *http.Request) {
	videos, err := h.streamManager.GetStorage().GetAvailableVideos(r.Context())
	if err != nil {
		h.logger.Error("listVideosHandler", "handlers.go", fmt.Sprintf("Failed to get videos: %v", err))
		http.Error(w, fmt.Sprintf("Failed to get videos: %v", err), http.StatusInternalServerError)
		return
	}

	var response []VideoResponse
	for _, video := range videos {
		// Получаем метаданные
		meta, err := h.streamManager.GetStorage().GetVideoMetadata(r.Context(), int64(video.ID))
		if err != nil {
			h.logger.Error("listVideosHandler", "handlers.go", fmt.Sprintf("Failed to get metadata for video %d: %v", video.ID, err))
			continue
		}

		// Получаем HLS-плейлист
		_, err = h.streamManager.GetStorage().GetHLSPlaylist(r.Context(), int64(video.ID))
		if err != nil {
			h.logger.Error("listVideosHandler", "handlers.go", fmt.Sprintf("Failed to get HLS for video %d: %v", video.ID, err))
			continue
		}

		// Получаем миниатюру
		thumb, err := h.streamManager.GetStorage().GetThumbnail(r.Context(), int64(video.ID))
		if err != nil {
			h.logger.Error("listVideosHandler", "handlers.go", fmt.Sprintf("Failed to get thumbnail for video %d: %v", video.ID, err))
			continue
		}

		// Формируем URL для HLS и миниатюры
		hlsURL := fmt.Sprintf("/video/%d/hls/playlist.m3u8", video.ID)
		thumbnailURL := fmt.Sprintf("/thumbnails/%s", filepath.Base(thumb.FilePath))

		response = append(response, VideoResponse{
			ID:           int64(video.ID),
			Title:        video.Title,
			FilePath:     video.FilePath,
			HLSURL:       hlsURL,
			ThumbnailURL: thumbnailURL,
			Duration:     meta.Duration,
			Status:       video.Status,
			CreatedAt:    video.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("listVideosHandler", "handlers.go", fmt.Sprintf("Failed to encode response: %v", err))
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

// VideoHLSHandler обрабатывает запросы к /video/{video_id}/hls/*
func (h *Handler) VideoHLSHandler(w http.ResponseWriter, r *http.Request) {
	// Извлекаем video_id из URL
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}
	videoIDStr := pathParts[2]
	videoID, err := strconv.ParseInt(videoIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid video ID", http.StatusBadRequest)
		return
	}

	// Получаем HLS-плейлист из базы данных
	hls, err := h.streamManager.GetStorage().GetHLSPlaylist(r.Context(), videoID)
	if err != nil {
		h.logger.Error("videoHLSHandler", "handlers.go", fmt.Sprintf("Failed to get HLS playlist for video %d: %v", videoID, err))
		http.Error(w, fmt.Sprintf("Failed to get HLS playlist: %v", err), http.StatusInternalServerError)
		return
	}

	// Определяем, запрашивается ли плейлист или сегмент
	requestedPath := filepath.Join(filepath.Dir(hls.PlaylistPath), filepath.Base(r.URL.Path))
	if _, err := os.Stat(requestedPath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, requestedPath)
}

// ThumbnailHandler обрабатывает запросы к /thumbnails/*
func (h *Handler) ThumbnailHandler(w http.ResponseWriter, r *http.Request) {
	thumbnailName := filepath.Base(r.URL.Path)
	thumbnailPath := filepath.Join(h.streamManager.GetConfig().ThumbnailDir, thumbnailName)
	if _, err := os.Stat(thumbnailPath); os.IsNotExist(err) {
		http.Error(w, "Thumbnail not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, thumbnailPath)
}
