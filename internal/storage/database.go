package storage

import (
	"context"
	"fmt"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/utils"

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

// SaveStreamMetadata сохраняет метаданные стрима в таблицу stream_metadata
func (s *Storage) SaveStreamMetadata(ctx context.Context, meta *database.StreamMetadata) error {
	query := `
		INSERT INTO stream_metadata (stream_id, duration, resolution, format, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (stream_id) DO NOTHING
	`
	_, err := s.db.Exec(ctx, query, meta.StreamID, meta.Duration, meta.Resolution, meta.Format, meta.CreatedAt)
	if err != nil {
		s.logger.Errorf("SaveStreamMetadata", "storage.go", "Failed to save stream metadata for stream ID %s: %v", meta.StreamID, err)
		return fmt.Errorf("failed to save stream metadata: %w", err)
	}
	s.logger.Infof("SaveStreamMetadata", "storage.go", "Stream metadata saved for stream ID: %s", meta.StreamID)
	return nil
}

// UpdateStreamMetadataStatus обновляет продолжительность стрима в таблице stream_metadata
func (s *Storage) UpdateStreamMetadataStatus(ctx context.Context, streamID string, duration int) error {
	query := `
		UPDATE stream_metadata
		SET duration = $2
		WHERE stream_id = $1
	`
	_, err := s.db.Exec(ctx, query, streamID, duration)
	if err != nil {
		s.logger.Errorf("UpdateStreamMetadataStatus", "storage.go", "Failed to update stream metadata duration for stream ID %s: %v", streamID, err)
		return fmt.Errorf("failed to update stream metadata duration: %w", err)
	}
	s.logger.Infof("UpdateStreamMetadataStatus", "storage.go", "Stream metadata duration updated for stream ID: %s", streamID)
	return nil
}

// SaveHLSMerkleProof сохраняет Merkle-доказательство для HLS-сегмента в таблицу hls_merkle_proofs
func (s *Storage) SaveHLSMerkleProof(ctx context.Context, proof *database.HLSMerkleProof) error {
	query := `
		INSERT INTO hls_merkle_proofs (stream_id, segment_index, proof_path, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := s.db.Exec(ctx, query, proof.StreamID, proof.SegmentIndex, proof.ProofPath, proof.CreatedAt)
	if err != nil {
		s.logger.Errorf("SaveHLSMerkleProof", "storage.go", "Failed to save HLS Merkle proof for stream ID %s, segment index %d: %v", proof.StreamID, proof.SegmentIndex, err)
		return fmt.Errorf("failed to save HLS Merkle proof: %w", err)
	}
	s.logger.Infof("SaveHLSMerkleProof", "storage.go", "HLS Merkle proof saved for stream ID %s, segment index: %d", proof.StreamID, proof.SegmentIndex)
	return nil
}

// SaveHLSPlaylist сохраняет информацию о HLS-плейлисте в таблицу hls_playlists
func (s *Storage) SaveHLSPlaylist(ctx context.Context, hls *database.HLSPlaylist) error {
	query := `
		INSERT INTO hls_playlists (stream_id, playlist_path, created_at)
		VALUES ($1, $2, $3)
		RETURNING id
	`
	err := s.db.QueryRow(ctx, query, hls.StreamID, hls.PlaylistPath, hls.CreatedAt).Scan(&hls.ID)
	if err != nil {
		s.logger.Errorf("SaveHLSPlaylist", "storage.go", "Failed to save HLS playlist for stream ID %s: %v", hls.StreamID, err)
		return fmt.Errorf("failed to save HLS playlist: %w", err)
	}
	s.logger.Infof("SaveHLSPlaylist", "storage.go", "HLS playlist saved for stream ID: %s", hls.StreamID)
	return nil
}

// GetHLSPlaylist получает информацию о HLS-плейлисте по stream_id
func (s *Storage) GetHLSPlaylist(ctx context.Context, streamID string) (*database.HLSPlaylist, error) {
	query := `
		SELECT id, stream_id, playlist_path, created_at
		FROM hls_playlists
		WHERE stream_id = $1
	`
	var hls database.HLSPlaylist
	err := s.db.QueryRow(ctx, query, streamID).Scan(
		&hls.ID, &hls.StreamID, &hls.PlaylistPath, &hls.CreatedAt,
	)
	if err != nil {
		s.logger.Errorf("GetHLSPlaylist", "storage.go", "Failed to get HLS playlist for stream ID %s: %v", streamID, err)
		return nil, fmt.Errorf("failed to get HLS playlist: %w", err)
	}
	return &hls, nil
}

// SaveProcessingLog сохраняет лог обработки в таблицу processing_logs
func (s *Storage) SaveProcessingLog(ctx context.Context, logEntry *database.ProcessingLog) error {
	query := `
		INSERT INTO processing_logs (stream_id, log_message, log_level, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := s.db.Exec(ctx, query, logEntry.StreamID, logEntry.LogMessage, logEntry.LogLevel, logEntry.CreatedAt)
	if err != nil {
		s.logger.Errorf("SaveProcessingLog", "storage.go", "Failed to save processing log for stream ID %s: %v", logEntry.StreamID, err)
		return fmt.Errorf("failed to save processing log: %w", err)
	}
	s.logger.Infof("SaveProcessingLog", "storage.go", "Processing log saved for stream ID: %s", logEntry.StreamID)
	return nil
}

// SaveArchiveEntry сохраняет информацию о завершённом стриме в таблицу archive
func (s *Storage) SaveArchiveEntry(ctx context.Context, archive *database.Archive) error {
	query := `
		INSERT INTO archive (stream_id, status, duration, hls_playlist_path, archived_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	err := s.db.QueryRow(ctx, query, archive.StreamID, archive.Status, archive.Duration, archive.HLSPlaylistPath, archive.ArchivedAt).Scan(&archive.ID)
	if err != nil {
		s.logger.Errorf("SaveArchiveEntry", "storage.go", "Failed to save archive entry for stream ID %s: %v", archive.StreamID, err)
		return fmt.Errorf("failed to save archive entry: %w", err)
	}
	s.logger.Infof("SaveArchiveEntry", "storage.go", "Archive entry saved for stream ID: %s", archive.StreamID)
	return nil
}

// GetArchiveEntry получает запись из таблицы archive по stream_id
func (s *Storage) GetArchiveEntry(ctx context.Context, streamID string) (*database.Archive, error) {
	query := `
		SELECT id, stream_id, status, duration, hls_playlist_path, archived_at
		FROM archive
		WHERE stream_id = $1
	`
	var archive database.Archive
	err := s.db.QueryRow(ctx, query, streamID).Scan(
		&archive.ID, &archive.StreamID, &archive.Status, &archive.Duration, &archive.HLSPlaylistPath, &archive.ArchivedAt,
	)
	if err != nil {
		s.logger.Errorf("GetArchiveEntry", "storage.go", "Failed to get archive entry for stream ID %s: %v", streamID, err)
		return nil, fmt.Errorf("failed to get archive entry: %w", err)
	}
	return &archive, nil
}

// GetAllArchiveEntries получает все записи из таблицы archive
func (s *Storage) GetAllArchiveEntries(ctx context.Context) ([]*database.Archive, error) {
	query := `
		SELECT id, stream_id, status, duration, hls_playlist_path, archived_at
		FROM archive
		ORDER BY archived_at DESC
	`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		s.logger.Errorf("GetAllArchiveEntries", "storage.go", "Failed to query archived streams: %v", err)
		return nil, fmt.Errorf("failed to query archived streams: %v", err)
	}
	defer rows.Close()

	var archives []*database.Archive
	for rows.Next() {
		var archive database.Archive
		if err := rows.Scan(
			&archive.ID, &archive.StreamID, &archive.Status, &archive.Duration, &archive.HLSPlaylistPath, &archive.ArchivedAt,
		); err != nil {
			s.logger.Errorf("GetAllArchiveEntries", "storage.go", "Failed to scan archived stream: %v", err)
			continue
		}
		archives = append(archives, &archive)
	}

	if err := rows.Err(); err != nil {
		s.logger.Errorf("GetAllArchiveEntries", "storage.go", "Error iterating archived streams: %v", err)
		return nil, fmt.Errorf("error iterating archived streams: %v", err)
	}

	return archives, nil
}

// GetStreamMetadata получает метаданные стрима по stream_id
func (s *Storage) GetStreamMetadata(ctx context.Context, streamID string) (*database.StreamMetadata, error) {
	query := `
		SELECT stream_id, duration, resolution, format, created_at
		FROM stream_metadata
		WHERE stream_id = $1
	`
	var meta database.StreamMetadata
	err := s.db.QueryRow(ctx, query, streamID).Scan(
		&meta.StreamID, &meta.Duration, &meta.Resolution, &meta.Format, &meta.CreatedAt,
	)
	if err != nil {
		s.logger.Errorf("GetStreamMetadata", "storage.go", "Failed to get stream metadata for stream ID %s: %v", streamID, err)
		return nil, fmt.Errorf("failed to get stream metadata: %w", err)
	}
	return &meta, nil
}
