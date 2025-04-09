package database

import "time"

// Video представляет запись в таблице videos
type Video struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	FilePath  string    `json:"file_path"`
	Status    string    `json:"status"` // pending, processing, completed, failed
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// VideoMetadata представляет запись в таблице video_metadata
type VideoMetadata struct {
	ID         int       `json:"id"`
	VideoID    int       `json:"video_id"`
	Duration   int       `json:"duration"`    // в секундах
	Resolution string    `json:"resolution"`  // например, "1920x1080"
	Format     string    `json:"format"`      // например, "mp4"
	FileSize   int64     `json:"file_size"`   // в байтах
	MerkleRoot string    `json:"merkle_root"` // Корневой хэш дерева Меркла
	BlockCount int       `json:"block_count"` // Количество блоков
	CreatedAt  time.Time `json:"created_at"`
}

// Thumbnail представляет запись в таблице thumbnails
type Thumbnail struct {
	ID        int       `json:"id"`
	VideoID   int       `json:"video_id"`
	FilePath  string    `json:"file_path"`
	CreatedAt time.Time `json:"created_at"`
}

// ProcessingLog представляет запись в таблице processing_logs
type ProcessingLog struct {
	ID         int       `json:"id"`
	VideoID    int       `json:"video_id"`
	LogMessage string    `json:"log_message"`
	LogLevel   string    `json:"log_level"` // info, warning, error
	CreatedAt  time.Time `json:"created_at"`
}

// MerkleProof представляет запись в таблице merkle_proofs
type MerkleProof struct {
	ID         int       `json:"id"`
	VideoID    int       `json:"video_id"`
	BlockIndex int       `json:"block_index"`
	ProofPath  string    `json:"proof_path"` // Сериализованный путь доказательства
	CreatedAt  time.Time `json:"created_at"`
}

type HLSPlaylist struct {
	ID           int64     `json:"id"`
	VideoID      int64     `json:"video_id"`
	PlaylistPath string    `json:"playlist_path"`
	CreatedAt    time.Time `json:"created_at"`
}

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
