package storage

import (
	"context"
	"fmt"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/utils"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Storage предоставляет методы для работы с базой данных
type Storage struct {
	db     *pgxpool.Pool
	logger *utils.Logger
}

// NewStorage создает новый экземпляр Storage
func NewStorage(db *pgxpool.Pool, logger *utils.Logger) *Storage {
	return &Storage{
		db:     db,
		logger: logger,
	}
}

// Ping проверяет подключение к базе данных
func (s *Storage) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

// SaveVideo сохраняет информацию о видео в таблицу videos
func (s *Storage) SaveVideo(ctx context.Context, video *database.Video) (int, error) {
	query := `
		INSERT INTO videos (title, file_path, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	var id int
	err := s.db.QueryRow(ctx, query, video.Title, video.FilePath, video.Status, video.CreatedAt, video.UpdatedAt).Scan(&id)
	if err != nil {
		s.logger.Errorf("SaveVideo", "database.go", "Failed to save video: %v", err)
		return 0, fmt.Errorf("failed to save video: %w", err)
	}
	s.logger.Infof("SaveVideo", "database.go", "Video saved with ID: %d", id)
	return id, nil
}

// SaveVideoMetadata сохраняет метаданные видео в таблицу video_metadata
func (s *Storage) SaveVideoMetadata(ctx context.Context, meta *database.VideoMetadata) error {
	query := `
        INSERT INTO video_metadata (video_id, duration, resolution, format, file_size, block_count, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id
    `
	err := s.db.QueryRow(ctx, query,
		meta.VideoID, meta.Duration, meta.Resolution, meta.Format, meta.FileSize, meta.BlockCount, meta.CreatedAt,
	).Scan(&meta.ID)
	if err != nil {
		return fmt.Errorf("failed to save video metadata: %w", err)
	}
	return nil
}

// GetVideoStatus retrieves the status of a video by its ID.
func (s *Storage) GetVideoStatus(ctx context.Context, videoID int64) (string, error) {
	var status string
	query := `
        SELECT status
        FROM videos
        WHERE id = $1`
	err := s.db.QueryRow(ctx, query, videoID).Scan(&status)
	if err != nil {
		s.logger.Errorf("GetVideoStatus", "database.go", "Failed to get video status: %v", err)
		return "", err
	}
	return status, nil
}

// SaveThumbnail сохраняет информацию о миниатюре в таблицу thumbnails
func (s *Storage) SaveThumbnail(ctx context.Context, thumb *database.Thumbnail) error {
	query := `
		INSERT INTO thumbnails (video_id, file_path, created_at)
		VALUES ($1, $2, $3)
	`
	_, err := s.db.Exec(ctx, query, thumb.VideoID, thumb.FilePath, thumb.CreatedAt)
	if err != nil {
		s.logger.Errorf("SaveThumbnail", "database.go", "Failed to save thumbnail: %v", err)
		return fmt.Errorf("failed to save thumbnail: %w", err)
	}
	s.logger.Infof("SaveThumbnail", "database.go", "Thumbnail saved for video ID: %d", thumb.VideoID)
	return nil
}

// SaveProcessingLog сохраняет лог обработки в таблицу processing_logs
func (s *Storage) SaveProcessingLog(ctx context.Context, logEntry *database.ProcessingLog) error {
	query := `
		INSERT INTO processing_logs (video_id, log_message, log_level, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := s.db.Exec(ctx, query, logEntry.VideoID, logEntry.LogMessage, logEntry.LogLevel, logEntry.CreatedAt)
	if err != nil {
		s.logger.Errorf("SaveProcessingLog", "database.go", "Failed to save processing log: %v", err)
		return fmt.Errorf("failed to save processing log: %w", err)
	}
	s.logger.Infof("SaveProcessingLog", "database.go", "Processing log saved for video ID: %d", logEntry.VideoID)
	return nil
}

// UpdateVideoStatus обновляет статус видео в таблице videos
func (s *Storage) UpdateVideoStatus(ctx context.Context, videoID int, status string) error {
	query := `
		UPDATE videos
		SET status = $1, updated_at = $2
		WHERE id = $3
	`
	_, err := s.db.Exec(ctx, query, status, time.Now(), videoID)
	if err != nil {
		s.logger.Errorf("UpdateVideoStatus", "database.go", "Failed to update video status: %v", err)
		return fmt.Errorf("failed to update video status: %w", err)
	}
	s.logger.Infof("UpdateVideoStatus", "database.go", "Video status updated to %s for video ID: %d", status, videoID)
	return nil
}

// SaveMerkleProof сохраняет доказательство включения в таблицу merkle_proofs
func (s *Storage) SaveMerkleProof(ctx context.Context, proof *database.MerkleProof) error {
	query := `
		INSERT INTO merkle_proofs (video_id, block_index, proof_path, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := s.db.Exec(ctx, query, proof.VideoID, proof.BlockIndex, proof.ProofPath, proof.CreatedAt)
	if err != nil {
		s.logger.Errorf("SaveMerkleProof", "database.go", "Failed to save Merkle proof: %v", err)
		return fmt.Errorf("failed to save Merkle proof: %w", err)
	}
	s.logger.Infof("SaveMerkleProof", "database.go", "Merkle proof saved for video ID: %d, block index: %d", proof.VideoID, proof.BlockIndex)
	return nil
}

func (s *Storage) SaveHLSPlaylist(ctx context.Context, hls *database.HLSPlaylist) error {
	query := `
        INSERT INTO hls_playlists (video_id, playlist_path, created_at)
        VALUES ($1, $2, $3)
        RETURNING id
    `
	err := s.db.QueryRow(ctx, query, hls.VideoID, hls.PlaylistPath, hls.CreatedAt).Scan(&hls.ID)
	if err != nil {
		return fmt.Errorf("failed to save HLS playlist: %w", err)
	}
	return nil
}

func (s *Storage) GetAvailableVideos(ctx context.Context) ([]*database.Video, error) {
	query := `
        SELECT v.id, v.title, v.file_path, v.status, v.created_at
        FROM videos v
        WHERE v.status IN ('completed', 'canceled')
        ORDER BY v.created_at DESC
    `
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query videos: %w", err)
	}
	defer rows.Close()

	var videos []*database.Video
	for rows.Next() {
		var v database.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.FilePath, &v.Status, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan video: %w", err)
		}
		videos = append(videos, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating videos: %w", err)
	}
	return videos, nil
}

func (s *Storage) GetVideoMetadata(ctx context.Context, videoID int64) (*database.VideoMetadata, error) {
	query := `
        SELECT video_id, duration, resolution, format, file_size, merkle_root, block_count, created_at
        FROM video_metadata
        WHERE video_id = $1
    `
	var meta database.VideoMetadata
	err := s.db.QueryRow(ctx, query, videoID).Scan(
		&meta.VideoID, &meta.Duration, &meta.Resolution, &meta.Format,
		&meta.FileSize, &meta.MerkleRoot, &meta.BlockCount, &meta.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get video metadata: %w", err)
	}
	return &meta, nil
}

func (s *Storage) GetHLSPlaylist(ctx context.Context, videoID int64) (*database.HLSPlaylist, error) {
	query := `
        SELECT id, video_id, playlist_path, created_at
        FROM hls_playlists
        WHERE video_id = $1
    `
	var hls database.HLSPlaylist
	err := s.db.QueryRow(ctx, query, videoID).Scan(
		&hls.ID, &hls.VideoID, &hls.PlaylistPath, &hls.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get HLS playlist: %w", err)
	}
	return &hls, nil
}

func (s *Storage) GetThumbnail(ctx context.Context, videoID int64) (*database.Thumbnail, error) {
	query := `
        SELECT video_id, file_path, created_at
        FROM thumbnails
        WHERE video_id = $1
    `
	var thumb database.Thumbnail
	err := s.db.QueryRow(ctx, query, videoID).Scan(
		&thumb.VideoID, &thumb.FilePath, &thumb.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get thumbnail: %w", err)
	}
	return &thumb, nil
}
