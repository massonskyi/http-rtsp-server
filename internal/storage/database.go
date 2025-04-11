package storage

import (
	"context"
	"fmt"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/utils"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Storage предоставляет методы для работы с базой данных
type Storage struct {
	pool   *pgxpool.Pool
	logger *utils.Logger
}

// NewStorage создает новый экземпляр Storage
func NewStorage(pool *pgxpool.Pool, logger *utils.Logger) *Storage {
	return &Storage{
		pool:   pool,
		logger: logger,
	}
}

// Ping проверяет подключение к базе данных
func (s *Storage) Ping(ctx context.Context) error {
	err := s.pool.Ping(ctx)
	if err != nil {
		s.logger.Error("Ping", "storage.go", fmt.Sprintf("Failed to ping database: %v", err))
	}
	return err
}

// SaveStreamMetadata сохраняет метаданные стрима
const saveStreamMetadataQuery = `
	INSERT INTO stream_metadata (stream_id, stream_name, duration, resolution, format, created_at, preview_path)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (stream_id) DO UPDATE
	SET stream_name = $2, duration = $3, resolution = $4, format = $5, created_at = $6, preview_path = $7
`

func (s *Storage) SaveStreamMetadata(ctx context.Context, meta *database.StreamMetadata) error {
	_, err := s.pool.Exec(ctx, saveStreamMetadataQuery,
		meta.StreamID,
		meta.StreamName,
		meta.Duration,
		meta.Resolution,
		meta.Format,
		meta.CreatedAt,
		meta.PreviewPath,
	)
	if err != nil {
		s.logger.Error("SaveStreamMetadata", "storage.go", fmt.Sprintf("Failed to save stream metadata for stream_id %s: %v", meta.StreamID, err))
		return fmt.Errorf("failed to save stream metadata: %w", err)
	}
	s.logger.Info("SaveStreamMetadata", "storage.go", fmt.Sprintf("Saved stream metadata for stream_id %s", meta.StreamID))
	return nil
}

// UpdateStreamMetadata обновляет метаданные стрима
const updateStreamMetadataQuery = `
	UPDATE stream_metadata
	SET duration = $2, resolution = $3, format = $4, preview_path = $5
	WHERE stream_id = $1
`

func (s *Storage) UpdateStreamMetadata(ctx context.Context, meta *database.StreamMetadata) error {
	_, err := s.pool.Exec(ctx, updateStreamMetadataQuery,
		meta.StreamID,
		meta.Duration,
		meta.Resolution,
		meta.Format,
		meta.PreviewPath,
	)
	if err != nil {
		s.logger.Error("UpdateStreamMetadata", "storage.go", fmt.Sprintf("Failed to update stream metadata for stream_id %s: %v", meta.StreamID, err))
		return fmt.Errorf("failed to update stream metadata: %w", err)
	}
	s.logger.Info("UpdateStreamMetadata", "storage.go", fmt.Sprintf("Updated stream metadata for stream_id %s", meta.StreamID))
	return nil
}

// GetStreamMetadata получает метаданные стрима по stream_id
const getStreamMetadataQuery = `
	SELECT stream_id, stream_name, duration, resolution, format, created_at, preview_path
	FROM stream_metadata
	WHERE stream_id = $1
`

func (s *Storage) GetStreamMetadata(ctx context.Context, streamID string) (*database.StreamMetadata, error) {
	var meta database.StreamMetadata
	err := s.pool.QueryRow(ctx, getStreamMetadataQuery, streamID).Scan(
		&meta.StreamID,
		&meta.StreamName,
		&meta.Duration,
		&meta.Resolution,
		&meta.Format,
		&meta.CreatedAt,
		&meta.PreviewPath,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.logger.Warning("GetStreamMetadata", "storage.go", fmt.Sprintf("Stream metadata not found for stream_id %s", streamID))
			return nil, fmt.Errorf("stream metadata not found for stream_id %s", streamID)
		}
		s.logger.Error("GetStreamMetadata", "storage.go", fmt.Sprintf("Failed to get stream metadata for stream_id %s: %v", streamID, err))
		return nil, fmt.Errorf("failed to get stream metadata: %w", err)
	}
	return &meta, nil
}

// GetStreamMetadataByName получает метаданные стрима по stream_name
const getStreamMetadataByNameQuery = `
	SELECT stream_id, stream_name, duration, resolution, format, created_at, preview_path
	FROM stream_metadata
	WHERE stream_name = $1
	ORDER BY created_at DESC
	LIMIT 1
`

func (s *Storage) GetStreamMetadataByName(ctx context.Context, streamName string) (*database.StreamMetadata, error) {
	var meta database.StreamMetadata
	err := s.pool.QueryRow(ctx, getStreamMetadataByNameQuery, streamName).Scan(
		&meta.StreamID,
		&meta.StreamName,
		&meta.Duration,
		&meta.Resolution,
		&meta.Format,
		&meta.CreatedAt,
		&meta.PreviewPath,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.logger.Warning("GetStreamMetadataByName", "storage.go", fmt.Sprintf("Stream metadata not found for stream_name %s", streamName))
			return nil, fmt.Errorf("stream metadata not found for stream_name %s", streamName)
		}
		s.logger.Error("GetStreamMetadataByName", "storage.go", fmt.Sprintf("Failed to get stream metadata for stream_name %s: %v", streamName, err))
		return nil, fmt.Errorf("failed to get stream metadata by name: %w", err)
	}
	return &meta, nil
}

// SaveProcessingLog сохраняет лог обработки
const saveProcessingLogQuery = `
	INSERT INTO processing_logs (stream_id, stream_name, log_message, log_level, created_at)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id
`

func (s *Storage) SaveProcessingLog(ctx context.Context, log *database.ProcessingLog) error {
	err := s.pool.QueryRow(ctx, saveProcessingLogQuery,
		log.StreamID,
		log.StreamName,
		log.LogMessage,
		log.LogLevel,
		log.CreatedAt,
	).Scan(&log.ID)
	if err != nil {
		s.logger.Error("SaveProcessingLog", "storage.go", fmt.Sprintf("Failed to save processing log for stream_id %s: %v", log.StreamID, err))
		return fmt.Errorf("failed to save processing log: %w", err)
	}
	s.logger.Info("SaveProcessingLog", "storage.go", fmt.Sprintf("Saved processing log for stream_id %s, log_id %d", log.StreamID, log.ID))
	return nil
}

// SaveHLSPlaylist сохраняет информацию о HLS-плейлисте
const saveHLSPlaylistQuery = `
	INSERT INTO hls_playlists (stream_id, stream_name, playlist_path, created_at)
	VALUES ($1, $2, $3, $4)
	RETURNING id
`

func (s *Storage) SaveHLSPlaylist(ctx context.Context, playlist *database.HLSPlaylist) error {
	err := s.pool.QueryRow(ctx, saveHLSPlaylistQuery,
		playlist.StreamID,
		playlist.StreamName,
		playlist.PlaylistPath,
		playlist.CreatedAt,
	).Scan(&playlist.ID)
	if err != nil {
		s.logger.Error("SaveHLSPlaylist", "storage.go", fmt.Sprintf("Failed to save HLS playlist for stream_id %s: %v", playlist.StreamID, err))
		return fmt.Errorf("failed to save HLS playlist: %w", err)
	}
	s.logger.Info("SaveHLSPlaylist", "storage.go", fmt.Sprintf("Saved HLS playlist for stream_id %s, playlist_id %d", playlist.StreamID, playlist.ID))
	return nil
}

// SaveHLSMerkleProof сохраняет доказательство Merkle для HLS-сегмента
const saveHLSMerkleProofQuery = `
	INSERT INTO hls_merkle_proofs (stream_id, stream_name, segment_index, proof_path, created_at)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id
`

func (s *Storage) SaveHLSMerkleProof(ctx context.Context, proof *database.HLSMerkleProof) error {
	err := s.pool.QueryRow(ctx, saveHLSMerkleProofQuery,
		proof.StreamID,
		proof.StreamName,
		proof.SegmentIndex,
		proof.ProofPath,
		proof.CreatedAt,
	).Scan(&proof.ID)
	if err != nil {
		s.logger.Error("SaveHLSMerkleProof", "storage.go", fmt.Sprintf("Failed to save HLS Merkle proof for stream_id %s, segment_index %d: %v", proof.StreamID, proof.SegmentIndex, err))
		return fmt.Errorf("failed to save HLS Merkle proof: %w", err)
	}
	s.logger.Info("SaveHLSMerkleProof", "storage.go", fmt.Sprintf("Saved HLS Merkle proof for stream_id %s, segment_index %d, proof_id %d", proof.StreamID, proof.SegmentIndex, proof.ID))
	return nil
}

// ArchiveStream архивирует стрим
const archiveStreamQuery = `
	INSERT INTO archive (stream_id, stream_name, status, duration, hls_playlist_path, archived_at)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (stream_id) DO NOTHING
	RETURNING id
`

func (s *Storage) ArchiveStream(ctx context.Context, archive *database.Archive) error {
	err := s.pool.QueryRow(ctx, archiveStreamQuery,
		archive.StreamID,
		archive.StreamName,
		archive.Status,
		archive.Duration,
		archive.HLSPlaylistPath,
		archive.ArchivedAt,
	).Scan(&archive.ID)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.logger.Info("ArchiveStream", "storage.go", fmt.Sprintf("Stream %s is already archived, skipping", archive.StreamID))
			return nil // Запись уже существует, дубликат предотвращён
		}
		s.logger.Error("ArchiveStream", "storage.go", fmt.Sprintf("Failed to archive stream %s: %v", archive.StreamID, err))
		return fmt.Errorf("failed to archive stream: %w", err)
	}
	s.logger.Info("ArchiveStream", "storage.go", fmt.Sprintf("Archived stream %s, archive_id %d", archive.StreamID, archive.ID))
	return nil
}

// GetArchiveEntry получает архивную запись по stream_id
const getArchiveEntryQuery = `
	SELECT id, stream_id, stream_name, status, duration, hls_playlist_path, archived_at
	FROM archive
	WHERE stream_id = $1
`

func (s *Storage) GetArchiveEntry(ctx context.Context, streamID string) (*database.Archive, error) {
	var archive database.Archive
	err := s.pool.QueryRow(ctx, getArchiveEntryQuery, streamID).Scan(
		&archive.ID,
		&archive.StreamID,
		&archive.StreamName,
		&archive.Status,
		&archive.Duration,
		&archive.HLSPlaylistPath,
		&archive.ArchivedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.logger.Warning("GetArchiveEntry", "storage.go", fmt.Sprintf("Archive entry not found for stream_id %s", streamID))
			return nil, fmt.Errorf("archive entry not found for stream_id %s", streamID)
		}
		s.logger.Error("GetArchiveEntry", "storage.go", fmt.Sprintf("Failed to get archive entry for stream_id %s: %v", streamID, err))
		return nil, fmt.Errorf("failed to get archive entry: %w", err)
	}
	return &archive, nil
}

// GetArchiveEntryByName получает архивную запись по stream_name
const getArchiveEntryByNameQuery = `
	SELECT id, stream_id, stream_name, status, duration, hls_playlist_path, archived_at
	FROM archive
	WHERE stream_name = $1
	ORDER BY archived_at DESC
	LIMIT 1
`

func (s *Storage) GetArchiveEntryByName(ctx context.Context, streamName string) (*database.Archive, error) {
	var archive database.Archive
	err := s.pool.QueryRow(ctx, getArchiveEntryByNameQuery, streamName).Scan(
		&archive.ID,
		&archive.StreamID,
		&archive.StreamName,
		&archive.Status,
		&archive.Duration,
		&archive.HLSPlaylistPath,
		&archive.ArchivedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.logger.Warningf("GetArchiveEntryByName", "storage.go", "Archive entry not found for stream_name %s", streamName)
			return nil, fmt.Errorf("archive entry not found for stream_name %s", streamName)
		}
		s.logger.Error("GetArchiveEntryByName", "storage.go", fmt.Sprintf("Failed to get archive entry for stream_name %s: %v", streamName, err))
		return nil, fmt.Errorf("failed to get archive entry by name: %w", err)
	}
	return &archive, nil
}

// GetAllArchiveEntries получает все архивные записи
const getAllArchiveEntriesQuery = `
	SELECT id, stream_id, stream_name, status, duration, hls_playlist_path, archived_at
	FROM archive
`

func (s *Storage) GetAllArchiveEntries(ctx context.Context) ([]*database.Archive, error) {
	rows, err := s.pool.Query(ctx, getAllArchiveEntriesQuery)
	if err != nil {
		s.logger.Error("GetAllArchiveEntries", "storage.go", fmt.Sprintf("Failed to get all archive entries: %v", err))
		return nil, fmt.Errorf("failed to get all archive entries: %w", err)
	}
	defer rows.Close()

	var archives []*database.Archive
	for rows.Next() {
		var archive database.Archive
		if err := rows.Scan(
			&archive.ID,
			&archive.StreamID,
			&archive.StreamName,
			&archive.Status,
			&archive.Duration,
			&archive.HLSPlaylistPath,
			&archive.ArchivedAt,
		); err != nil {
			s.logger.Error("GetAllArchiveEntries", "storage.go", fmt.Sprintf("Failed to scan archive entry: %v", err))
			return nil, fmt.Errorf("failed to scan archive entry: %w", err)
		}
		archives = append(archives, &archive)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("GetAllArchiveEntries", "storage.go", fmt.Sprintf("Error iterating archive entries: %v", err))
		return nil, fmt.Errorf("error iterating archive entries: %w", err)
	}

	return archives, nil
}
