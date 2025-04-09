-- Таблица видеофайлов (без user_id, так как авторизация не требуется)
CREATE TABLE IF NOT EXISTS videos (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL, -- Название видео
    file_path TEXT NOT NULL UNIQUE, -- Путь к файлу на диске
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')), -- Статус обработки
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Таблица метаданных видео
CREATE TABLE IF NOT EXISTS video_metadata (
    id SERIAL PRIMARY KEY,
    video_id INT NOT NULL REFERENCES videos(id) ON DELETE CASCADE, -- Связь с видео
    duration INT NOT NULL, -- Длительность в секундах
    resolution VARCHAR(20) NOT NULL, -- Разрешение (например, "1920x1080")
    format VARCHAR(10) NOT NULL, -- Формат (например, "mp4", "avi")
    file_size BIGINT NOT NULL, -- Размер файла в байтах
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Таблица миниатюр
CREATE TABLE IF NOT EXISTS thumbnails (
    id SERIAL PRIMARY KEY,
    video_id INT NOT NULL REFERENCES videos(id) ON DELETE CASCADE, -- Связь с видео
    file_path TEXT NOT NULL UNIQUE, -- Путь к миниатюре на диске
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS processing_logs (
	id SERIAL PRIMARY KEY,
	video_id INT NOT NULL REFERENCES videos(id) ON DELETE CASCADE, -- Связь с видео
	log_message TEXT NOT NULL, -- Сообщение лога
    log_level VARCHAR(10) NOT NULL CHECK (log_level IN ('info', 'warning', 'error')), -- Уровень лога
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
)

-- Индексы для оптимизации запросов
CREATE INDEX idx_videos_status ON videos(status);
CREATE INDEX idx_video_metadata_video_id ON video_metadata(video_id);
CREATE INDEX idx_thumbnails_video_id ON thumbnails(video_id);
CREATE INDEX idx_processing_logs_video_id ON processing_logs(video_id);

-- Триггер для обновления updated_at в таблице videos
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_videos_updated_at
    BEFORE UPDATE ON videos
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();