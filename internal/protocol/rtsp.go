package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/merkle"
	"rstp-rsmt-server/internal/storage"
	"rstp-rsmt-server/internal/utils"
	"time"
)

// RTSPClient управляет подключением к RTSP-потоку и его обработкой
type RTSPClient struct {
	cfg     *config.Config
	logger  *utils.Logger
	storage *storage.Storage
	fs      *storage.FileSystem
}

// NewRTSPClient создает новый экземпляр RTSPClient
func NewRTSPClient(cfg *config.Config, logger *utils.Logger, storage *storage.Storage, fs *storage.FileSystem) *RTSPClient {
	return &RTSPClient{
		cfg:     cfg,
		logger:  logger,
		storage: storage,
		fs:      fs,
	}
}

// ProcessStream подключается к RTSP-потоку, записывает видео и сохраняет метаданные
func (c *RTSPClient) ProcessStream(ctx context.Context, rtspURL string, streamID string) error {
	// Логируем начало обработки
	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Starting to process RTSP stream: %s", rtspURL))

	// Валидация RTSP-URL
	if err := c.validateRTSPURL(rtspURL); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Invalid RTSP URL: %v", err))
		return fmt.Errorf("invalid RTSP URL: %w", err)
	}

	// Проверяем доступность RTSP-потока с помощью FFmpeg
	if err := c.checkRTSPStream(ctx, rtspURL); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("RTSP stream is unavailable: %v", err))
		return fmt.Errorf("RTSP stream is unavailable: %w", err)
	}

	// Формируем имя файла для видео (сначала используем MKV)
	videoFilename := fmt.Sprintf("%s_%d.mkv", streamID, time.Now().Unix())
	videoFilePath := filepath.Join(c.cfg.VideoDir, videoFilename)
	finalVideoFilePath := filepath.Join(c.cfg.VideoDir, fmt.Sprintf("%s_%d.mp4", streamID, time.Now().Unix()))

	// Папка для HLS
	hlsDir := filepath.Join(c.cfg.VideoDir, fmt.Sprintf("%s_%d_hls", streamID, time.Now().Unix()))
	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to create HLS directory %s: %v", hlsDir, err))
		return fmt.Errorf("failed to create HLS directory: %w", err)
	}

	// Сохраняем запись о видео в базе данных со статусом "pending"
	video := &database.Video{
		Title:     streamID,
		FilePath:  finalVideoFilePath,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	videoID, err := c.storage.SaveVideo(ctx, video)
	if err != nil {
		return fmt.Errorf("failed to save video: %w", err)
	}

	// Обновляем статус на "processing"
	if err := c.storage.UpdateVideoStatus(ctx, videoID, "processing"); err != nil {
		return fmt.Errorf("failed to update video status: %w", err)
	}

	// Сохраняем лог обработки
	logEntry := &database.ProcessingLog{
		VideoID:    videoID,
		LogMessage: "Started processing RTSP stream",
		LogLevel:   "info",
		CreatedAt:  time.Now(),
	}
	if err := c.storage.SaveProcessingLog(ctx, logEntry); err != nil {
		return fmt.Errorf("failed to save processing log: %w", err)
	}

	// Каналы для координации этапов
	type recordResult struct {
		filePath string
		duration int
		err      error
	}
	type merkleResult struct {
		blocks [][]byte
		tree   *merkle.MerkleTree
		err    error
	}
	type thumbnailResult struct {
		filePath string
		err      error
	}
	type hlsResult struct {
		hlsPath string
		err     error
	}

	recordChan := make(chan recordResult)
	merkleChan := make(chan merkleResult)
	thumbnailChan := make(chan thumbnailResult)
	hlsChan := make(chan hlsResult)

	// Запоминаем время начала записи
	startTime := time.Now()

	// Этап 1: Запись видео в MKV
	go func() {
		defer func() {
			c.logger.Infof("ProcessStream", "rtsp.go", "FFmpeg recording process for stream %s completed", streamID)
		}()

		ffmpegCmd := exec.Command("ffmpeg",
			"-fflags", "+genpts",
			"-use_wallclock_as_timestamps", "1",
			"-rtsp_transport", "tcp",
			"-i", rtspURL,
			"-c:v", "libx264",
			"-preset", "fast",
			"-c:a", "aac",
			"-f", "matroska",
			"-y",
			videoFilePath,
		)

		var stderr bytes.Buffer
		ffmpegCmd.Stderr = &stderr
		ffmpegCmd.Stdout = &stderr

		// Для отладки записываем вывод FFmpeg в файл
		f, err := os.Create(fmt.Sprintf("ffmpeg_output_%s.log", streamID))
		if err == nil {
			ffmpegCmd.Stderr = f
			ffmpegCmd.Stdout = f
			defer f.Close()
		}

		// Запускаем FFmpeg
		if err := ffmpegCmd.Start(); err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to start FFmpeg: %v", err))
			recordChan <- recordResult{err: fmt.Errorf("failed to start FFmpeg: %w", err)}
			return
		}

		// Ожидаем либо завершения FFmpeg, либо отмены контекста
		done := make(chan error, 1)
		go func() {
			done <- ffmpegCmd.Wait()
		}()

		select {
		case <-ctx.Done():
			// При отмене контекста отправляем команду 'q' для мягкого завершения
			c.logger.Infof("ProcessStream", "rtsp.go", "Received cancellation, sending 'q' to FFmpeg for stream %s", streamID)
			ffmpegCmd.Stdin = bytes.NewReader([]byte("q"))
			// Даем FFmpeg время на завершение
			select {
			case err := <-done:
				if err != nil {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("FFmpeg exited with error after 'q': %v, FFmpeg output: %s", err, stderr.String()))
				} else {
					c.logger.Info("ProcessStream", "rtsp.go", "FFmpeg completed gracefully after 'q'")
				}
			case <-time.After(15 * time.Second):
				c.logger.Warning("ProcessStream", "rtsp.go", "FFmpeg did not exit within 15 seconds, killing process")
				if err := ffmpegCmd.Process.Kill(); err != nil {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to kill FFmpeg process: %v", err))
				}
			}

			// Проверяем файл с помощью ffprobe
			if err := c.checkVideoFile(videoFilePath); err != nil {
				c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Video file %s is corrupted: %v", videoFilePath, err))
				// Удаляем поврежденный файл
				if err := os.Remove(videoFilePath); err != nil && !os.IsNotExist(err) {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to remove corrupted video file %s: %v", videoFilePath, err))
				}
				duration := int(time.Since(startTime).Seconds())
				recordChan <- recordResult{filePath: videoFilePath, duration: duration, err: fmt.Errorf("video file is corrupted: %w", err)}
				return
			}

			// Вычисляем продолжительность записи
			duration := int(time.Since(startTime).Seconds())
			// Создаем новый контекст для обновления статуса
			newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Устанавливаем статус "canceled"
			if err := c.storage.UpdateVideoStatus(newCtx, videoID, "canceled"); err != nil {
				c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to update video status to canceled: %v", err))
			}
			// Сохраняем файл, так как остановка через /stop-stream легальна
			c.logger.Infof("ProcessStream", "rtsp.go", "Keeping video file %s after legal cancellation via /stop-stream", videoFilePath)
			// Проверяем, есть ли данные в файле
			fileSize := getFileSize(videoFilePath)
			if fileSize == 0 {
				c.logger.Warningf("ProcessStream", "rtsp.go", "Video file %s is empty after cancellation, skipping further processing", videoFilePath)
				recordChan <- recordResult{filePath: videoFilePath, duration: duration, err: fmt.Errorf("recording canceled and file is empty: %w", ctx.Err())}
				return
			}
			// Конвертируем MKV в MP4
			if err := c.convertMKVtoMP4(videoFilePath, finalVideoFilePath); err != nil {
				c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to convert MKV to MP4: %v", err))
				// Продолжаем обработку с MKV, если конвертация не удалась
				finalVideoFilePath = videoFilePath
			} else {
				// Удаляем промежуточный MKV файл
				if err := os.Remove(videoFilePath); err != nil && !os.IsNotExist(err) {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to remove intermediate MKV file %s: %v", videoFilePath, err))
				}
			}
			// Проверяем файл с помощью ffprobe
			if err := c.checkVideoFile(finalVideoFilePath); err != nil {
				c.logger.Warning("ProcessStream", "rtsp.go", fmt.Sprintf("Video file %s is corrupted but proceeding with processing: %v", finalVideoFilePath, err))
			}
			// Сохраняем файл, так как остановка через /stop-stream легальна
			c.logger.Infof("ProcessStream", "rtsp.go", "Keeping video file %s after legal cancellation via /stop-stream", finalVideoFilePath)
			// Если файл не пустой, продолжаем обработку
			recordChan <- recordResult{filePath: finalVideoFilePath, duration: duration, err: nil}
			return

		case err := <-done:
			// FFmpeg завершился сам
			duration := int(time.Since(startTime).Seconds())
			if err != nil {
				c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to record video with FFmpeg: %v, FFmpeg output: %s", err, stderr.String()))
				// Создаем новый контекст для обновления статуса
				newCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := c.storage.UpdateVideoStatus(newCtx, videoID, "failed"); err != nil {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to update video status to failed: %v", err))
				}
				// Удаляем файл при ошибке
				if err := os.Remove(videoFilePath); err != nil && !os.IsNotExist(err) {
					c.logger.Errorf("ProcessStream", "rtsp.go", "Failed to remove failed video file %s: %v", videoFilePath, err)
				} else {
					c.logger.Infof("ProcessStream", "rtsp.go", "Removed failed video file %s", videoFilePath)
				}
				recordChan <- recordResult{err: fmt.Errorf("failed to record video: %w, FFmpeg output: %s", err, stderr.String())}
				return
			}
			// Конвертируем MKV в MP4
			if err := c.convertMKVtoMP4(videoFilePath, finalVideoFilePath); err != nil {
				c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to convert MKV to MP4: %v", err))
				// Продолжаем обработку с MKV, если конвертация не удалась
				finalVideoFilePath = videoFilePath
			} else {
				// Удаляем промежуточный MKV файл
				if err := os.Remove(videoFilePath); err != nil && !os.IsNotExist(err) {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to remove intermediate MKV file %s: %v", videoFilePath, err))
				}
			}
			// Проверяем файл с помощью ffprobe
			if err := c.checkVideoFile(finalVideoFilePath); err != nil {
				c.logger.Warning("ProcessStream", "rtsp.go", fmt.Sprintf("Video file %s is corrupted but proceeding with processing: %v", finalVideoFilePath, err))
			}
			recordChan <- recordResult{filePath: finalVideoFilePath, duration: duration, err: nil}
		}
	}()

	// Ожидаем результат записи
	var duration int
	var newCtx context.Context
	var cancel context.CancelFunc
	// Убираем case <-ctx.Done(), так как recordChan уже обрабатывает отмену
	res := <-recordChan
	if res.err != nil {
		return res.err
	}
	videoFilePath = res.filePath
	duration = res.duration
	// Создаем новый контекст для дальнейших операций
	newCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Логируем продолжение обработки
	c.logger.Infof("ProcessStream", "rtsp.go", "Proceeding with post-processing for videoID %d", videoID)

	// Если статус "canceled", не меняем его на "completed"
	currentStatus, err := c.storage.GetVideoStatus(newCtx, int64(videoID))
	if err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to get video status: %v", err))
	} else if currentStatus != "canceled" {
		// Обновляем статус на "completed" только если не было отмены
		if err := c.storage.UpdateVideoStatus(newCtx, videoID, "completed"); err != nil {
			return fmt.Errorf("failed to update video status: %w", err)
		}
	}

	// Этап 2: Построение дерева Меркла
	go func() {
		c.logger.Infof("ProcessStream", "rtsp.go", "Starting Merkle tree construction for videoID %d", videoID)
		blocks, tree, err := c.buildMerkleTree(videoFilePath)
		merkleChan <- merkleResult{blocks: blocks, tree: tree, err: err}
	}()

	// Этап 3: Создание миниатюры
	go func() {
		c.logger.Infof("ProcessStream", "rtsp.go", "Starting thumbnail creation for videoID %d", videoID)
		thumbnailFilename := fmt.Sprintf("%s_%d.jpg", streamID, time.Now().Unix())
		thumbnailFilePath := filepath.Join(c.cfg.ThumbnailDir, thumbnailFilename)
		ffmpegThumbnailCmd := exec.Command("ffmpeg",
			"-i", videoFilePath,
			"-ss", "00:00:01",
			"-vframes", "1",
			"-y",
			thumbnailFilePath,
		)
		var stderr bytes.Buffer
		ffmpegThumbnailCmd.Stderr = &stderr
		err := ffmpegThumbnailCmd.Run()
		if err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to create thumbnail: %v, FFmpeg output: %s", err, stderr.String()))
			thumbnailChan <- thumbnailResult{err: fmt.Errorf("failed to create thumbnail: %w, FFmpeg output: %s", err, stderr.String())}
			return
		}
		thumbnailChan <- thumbnailResult{filePath: thumbnailFilePath, err: nil}
	}()

	// Этап 4: Генерация HLS
	go func() {
		c.logger.Infof("ProcessStream", "rtsp.go", "Starting HLS generation for videoID %d", videoID)
		hlsPath, err := c.generateHLS(videoFilePath, hlsDir)
		hlsChan <- hlsResult{hlsPath: hlsPath, err: err}
	}()

	// Ожидаем результаты построения дерева Меркла
	var blocks [][]byte
	var tree *merkle.MerkleTree
	select {
	case res := <-merkleChan:
		if res.err != nil {
			return res.err
		}
		blocks = res.blocks
		tree = res.tree
	case <-newCtx.Done():
		return newCtx.Err()
	}

	// Логируем перед сохранением метаданных
	c.logger.Infof("ProcessStream", "rtsp.go", "Preparing to save video metadata for videoID %d", videoID)

	// Проверяем подключение к базе данных
	if err := c.storage.Ping(newCtx); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Database connection failed: %v", err))
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Сохраняем метаданные видео
	meta := &database.VideoMetadata{
		VideoID:    videoID,
		Duration:   duration,
		Resolution: "1920x1080",
		Format:     "mp4",
		FileSize:   getFileSize(videoFilePath),
		MerkleRoot: fmt.Sprintf("%x", tree.Root.Hash),
		BlockCount: len(blocks),
		CreatedAt:  time.Now(),
	}
	if err := c.storage.SaveVideoMetadata(newCtx, meta); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save video metadata: %v", err))
		return fmt.Errorf("failed to save video metadata: %w", err)
	}
	c.logger.Infof("ProcessStream", "rtsp.go", "Successfully saved video metadata for videoID %d", videoID)

	// Генерируем и сохраняем доказательства включения
	for i := 0; i < len(blocks); i++ {
		proof, err := tree.GenerateProof(i)
		if err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to generate Merkle proof for block %d: %v", i, err))
			continue
		}

		proofPath, err := json.Marshal(proof.Path)
		if err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to serialize Merkle proof for block %d: %v", i, err))
			continue
		}

		merkleProof := &database.MerkleProof{
			VideoID:    videoID,
			BlockIndex: i,
			ProofPath:  string(proofPath),
			CreatedAt:  time.Now(),
		}
		if err := c.storage.SaveMerkleProof(newCtx, merkleProof); err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save Merkle proof for block %d: %v", i, err))
			continue
		}
	}

	// Ожидаем результат создания миниатюры
	var thumbnailFilePath string
	select {
	case res := <-thumbnailChan:
		if res.err != nil {
			return res.err
		}
		thumbnailFilePath = res.filePath
	case <-newCtx.Done():
		return newCtx.Err()
	}

	// Сохраняем информацию о миниатюре
	thumbnail := &database.Thumbnail{
		VideoID:   videoID,
		FilePath:  thumbnailFilePath,
		CreatedAt: time.Now(),
	}
	if err := c.storage.SaveThumbnail(newCtx, thumbnail); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save thumbnail: %v", err))
		return fmt.Errorf("failed to save thumbnail: %w", err)
	}

	// Ожидаем результат генерации HLS
	var hlsPath string
	select {
	case res := <-hlsChan:
		if res.err != nil {
			return res.err
		}
		hlsPath = res.hlsPath
	case <-newCtx.Done():
		return newCtx.Err()
	}

	// Сохраняем информацию о HLS в базе данных
	hlsPlaylist := &database.HLSPlaylist{
		VideoID:      int64(videoID),
		PlaylistPath: hlsPath,
		CreatedAt:    time.Now(),
	}
	if err := c.storage.SaveHLSPlaylist(newCtx, hlsPlaylist); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save HLS playlist: %v", err))
		return fmt.Errorf("failed to save HLS playlist: %w", err)
	}
	c.logger.Infof("ProcessStream", "rtsp.go", "HLS generated at %s for videoID %d", hlsPath, videoID)

	// Логируем успешное завершение
	logEntry = &database.ProcessingLog{
		VideoID:    videoID,
		LogMessage: "Successfully processed RTSP stream",
		LogLevel:   "info",
		CreatedAt:  time.Now(),
	}
	if err := c.storage.SaveProcessingLog(newCtx, logEntry); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save processing log: %v", err))
		return fmt.Errorf("failed to save processing log: %w", err)
	}

	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Successfully processed RTSP stream: %s", rtspURL))
	return nil
}

// convertMKVtoMP4 конвертирует MKV в MP4
func (c *RTSPClient) convertMKVtoMP4(inputPath, outputPath string) error {
	ffmpegCmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-movflags", "faststart",
		"-y",
		outputPath,
	)
	var stderr bytes.Buffer
	ffmpegCmd.Stderr = &stderr
	ffmpegCmd.Stdout = &stderr
	err := ffmpegCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to convert MKV to MP4: %w, FFmpeg output: %s", err, stderr.String())
	}
	return nil
}

// generateHLS генерирует HLS-сегменты и плейлист
func (c *RTSPClient) generateHLS(inputPath, hlsDir string) (string, error) {
	hlsPlaylist := filepath.Join(hlsDir, "playlist.m3u8")
	ffmpegCmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-hls_time", "10",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(hlsDir, "segment_%03d.ts"),
		"-y",
		hlsPlaylist,
	)
	var stderr bytes.Buffer
	ffmpegCmd.Stderr = &stderr
	ffmpegCmd.Stdout = &stderr
	err := ffmpegCmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to generate HLS: %w, FFmpeg output: %s", err, stderr.String())
	}
	return hlsPlaylist, nil
}

// validateRTSPURL проверяет корректность RTSP-URL и разрешение имени хоста
func (c *RTSPClient) validateRTSPURL(rtspURL string) error {
	// Парсим URL
	parsedURL, err := url.Parse(rtspURL)
	if err != nil {
		return fmt.Errorf("failed to parse RTSP URL: %w", err)
	}

	// Проверяем схему
	if parsedURL.Scheme != "rtsp" {
		return fmt.Errorf("URL scheme must be 'rtsp', got '%s'", parsedURL.Scheme)
	}

	// Проверяем наличие хоста
	if parsedURL.Host == "" {
		return fmt.Errorf("URL must contain a host")
	}

	// Проверяем разрешение имени хоста
	host := parsedURL.Hostname()
	_, err = net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname '%s': %w", host, err)
	}

	return nil
}

// checkRTSPStream проверяет доступность RTSP-потока с помощью FFmpeg
func (c *RTSPClient) checkRTSPStream(ctx context.Context, rtspURL string) error {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ffmpegCmd := exec.CommandContext(checkCtx, "ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-t", "1",
		"-f", "null",
		"-",
	)

	var stderr bytes.Buffer
	ffmpegCmd.Stderr = &stderr

	err := ffmpegCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to connect to RTSP stream: %w, FFmpeg output: %s", err, stderr.String())
	}

	return nil
}

// checkVideoFile проверяет, является ли видеофайл воспроизводимым с помощью ffprobe
func (c *RTSPClient) checkVideoFile(filePath string) error {
	ffprobeCmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_format",
		"-show_streams",
		filePath,
	)

	var stderr bytes.Buffer
	ffprobeCmd.Stderr = &stderr

	err := ffprobeCmd.Run()
	if err != nil {
		return fmt.Errorf("ffprobe failed: %w, output: %s", err, stderr.String())
	}
	return nil
}

// buildMerkleTree разделяет файл на блоки и строит дерево Меркла
func (c *RTSPClient) buildMerkleTree(filePath string) ([][]byte, *merkle.MerkleTree, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	const blockSize = 1024 * 1024
	var blocks [][]byte
	buffer := make([]byte, blockSize)

	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, nil, fmt.Errorf("failed to read file: %w", err)
		}
		if n == 0 {
			break
		}
		block := make([]byte, n)
		copy(block, buffer[:n])
		blocks = append(blocks, block)
	}

	tree, err := merkle.NewMerkleTree(blocks)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build Merkle tree: %w", err)
	}

	return blocks, tree, nil
}

// getFileSize возвращает размер файла в байтах
func getFileSize(filePath string) int64 {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0
	}
	return fileInfo.Size()
}
