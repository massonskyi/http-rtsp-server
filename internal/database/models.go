package database

import "time"

// StreamMetadata хранит метаданные стрима
type StreamMetadata struct {
	StreamID   string    `json:"stream_id"`
	Duration   int       `json:"duration"`
	Resolution string    `json:"resolution"`
	Format     string    `json:"format"`
	CreatedAt  time.Time `json:"created_at"`
}

// HLSMerkleProof хранит доказательства включения для HLS-сегментов
type HLSMerkleProof struct {
	ID           int       `json:"id"`
	StreamID     string    `json:"stream_id"`
	SegmentIndex int       `json:"segment_index"`
	ProofPath    string    `json:"proof_path"`
	CreatedAt    time.Time `json:"created_at"`
}

// HLSPlaylist хранит информацию о HLS-плейлисте
type HLSPlaylist struct {
	ID           int       `json:"id"`
	StreamID     string    `json:"stream_id"`
	PlaylistPath string    `json:"playlist_path"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProcessingLog хранит логи обработки
type ProcessingLog struct {
	ID         int       `json:"id"`
	StreamID   string    `json:"stream_id"`
	LogMessage string    `json:"log_message"`
	LogLevel   string    `json:"log_level"`
	CreatedAt  time.Time `json:"created_at"`
}

// Archive хранит информацию о завершённых стримах
type Archive struct {
	ID              int       `json:"id"`
	StreamID        string    `json:"stream_id"`
	Status          string    `json:"status"`
	Duration        int       `json:"duration"`
	HLSPlaylistPath string    `json:"hls_playlist_path"`
	ArchivedAt      time.Time `json:"archived_at"`
}
