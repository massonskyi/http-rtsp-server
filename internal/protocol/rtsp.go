package protocol

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"sort"
	"strings"
	"time"
)

// RTSPClient управляет подключением к RTSP-потоку и его обработкой
type RTSPClient struct {
	cfg     *config.Config
	logger  *utils.Logger
	storage *storage.Storage
	fs      *storage.FileSystem
}

// StreamInfo содержит информацию о потоках (видео и аудио)
type StreamInfo struct {
	HasVideo bool
	HasAudio bool
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

// checkStreamInfo проверяет наличие видео- и аудиопотоков в RTSP-потоке
func (c *RTSPClient) checkStreamInfo(ctx context.Context, rtspURL string) (StreamInfo, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ffprobeCmd := exec.CommandContext(checkCtx, "ffprobe",
		"-rtsp_transport", "tcp",
		"-show_streams",
		"-print_format", "json",
		rtspURL,
	)

	var stdout, stderr bytes.Buffer
	ffprobeCmd.Stdout = &stdout
	ffprobeCmd.Stderr = &stderr

	if err := ffprobeCmd.Run(); err != nil {
		return StreamInfo{}, fmt.Errorf("failed to probe RTSP stream: %w, ffprobe output: %s", err, stderr.String())
	}

	// Парсим JSON-вывод ffprobe
	var probeData struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &probeData); err != nil {
		return StreamInfo{}, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	info := StreamInfo{}
	for _, stream := range probeData.Streams {
		if stream.CodecType == "video" {
			info.HasVideo = true
		} else if stream.CodecType == "audio" {
			info.HasAudio = true
		}
	}

	if !info.HasVideo {
		return StreamInfo{}, fmt.Errorf("no video stream found in RTSP source")
	}

	return info, nil
}
func (c *RTSPClient) extractFirstFrame(ctx context.Context, rtspURL string, hlsDir string) (string, error) {
	previewPath := filepath.Join(hlsDir, "preview.jpg")

	// Используем FFmpeg для извлечения первого кадра
	ffmpegCmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", rtspURL,
		"-rtsp_transport", "tcp",
		"-vframes", "1", // Извлекаем только один кадр
		"-ss", "00:00:01", // Пропускаем первую секунду, чтобы получить качественный кадр
		"-f", "image2",
		previewPath,
	)

	var stderr bytes.Buffer
	ffmpegCmd.Stderr = &stderr
	ffmpegCmd.Stdout = &stderr

	if err := ffmpegCmd.Run(); err != nil {
		c.logger.Error("extractFirstFrame", "rtsp.go", fmt.Sprintf("Failed to extract first frame: %v, FFmpeg output: %s", err, stderr.String()))
		return "", fmt.Errorf("failed to extract first frame: %w, FFmpeg output: %s", err, stderr.String())
	}

	// Проверяем, что файл превью был создан
	if _, err := os.Stat(previewPath); os.IsNotExist(err) {
		return "", fmt.Errorf("preview file was not created at %s", previewPath)
	}

	c.logger.Info("extractFirstFrame", "rtsp.go", fmt.Sprintf("Successfully extracted first frame to %s", previewPath))
	return previewPath, nil
}

// ProcessStream обрабатывает RTSP-поток
// ProcessStream обрабатывает RTSP-поток
func (c *RTSPClient) ProcessStream(ctx context.Context, rtspURL string, streamID string, streamName string, hlsPath string) error {
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

	// Извлекаем первый кадр как превью
	hlsDir := filepath.Dir(hlsPath)
	previewPath, err := c.extractFirstFrame(ctx, rtspURL, hlsDir)
	if err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to extract preview for stream %s: %v", streamID, err))
		// Не прерываем выполнение, так как это не критично
	}

	// Проверяем наличие видео- и аудиопотоков
	streamInfo, err := c.checkStreamInfo(ctx, rtspURL)
	if err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to check stream info: %v", err))
		return fmt.Errorf("failed to check stream info: %w", err)
	}
	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Stream info: hasVideo=%v, hasAudio=%v", streamInfo.HasVideo, streamInfo.HasAudio))

	// Папка для HLS уже создана в StartStream, используем переданный hlsPath
	hlsPlaylist := hlsPath

	// Проверяем подключение к базе данных перед сохранением
	c.logger.Info("ProcessStream", "rtsp.go", "Checking database connection before saving metadata")
	if err := c.storage.Ping(ctx); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Database connection failed: %v", err))
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Сохраняем метаданные стрима в базе данных
	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Saving stream metadata for streamID %s", streamID))
	meta := &database.StreamMetadata{
		StreamID:    streamID,
		StreamName:  streamName,
		Duration:    0,
		Resolution:  "1920x1080",
		Format:      "hls",
		CreatedAt:   time.Now(),
		PreviewPath: previewPath, // Сохраняем путь к превью
	}
	if err := c.storage.SaveStreamMetadata(ctx, meta); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save stream metadata: %v", err))
		return fmt.Errorf("failed to save stream metadata: %w", err)
	}
	c.logger.Info("ProcessStream", "rtsp.go", "Stream metadata saved successfully")

	// Сохраняем лог обработки
	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Saving processing log for streamID %s", streamID))
	logEntry := &database.ProcessingLog{
		StreamID:   streamID,
		StreamName: streamName,
		LogMessage: "Started processing RTSP stream",
		LogLevel:   "info",
		CreatedAt:  time.Now(),
	}
	if err := c.storage.SaveProcessingLog(ctx, logEntry); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save processing log: %v", err))
		return fmt.Errorf("failed to save processing log: %w", err)
	}
	c.logger.Info("ProcessStream", "rtsp.go", "Processing log saved successfully")

	// Каналы для координации этапов
	type recordResult struct {
		duration int
		err      error
	}
	type merkleResult struct {
		blocks [][]byte
		tree   *merkle.MerkleTree
		err    error
	}

	recordChan := make(chan recordResult)
	merkleChan := make(chan merkleResult)

	// Запоминаем время начала записи
	startTime := time.Now()

	// Этап 1: Генерация HLS
	go func() {
		defer func() {
			c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("FFmpeg recording process for stream %s completed", streamID))
		}()

		// Формируем входные параметры
		inputParams := &InputParams{
			RTSPURL:       rtspURL,
			BufferSize:    "8192k",
			Timeout:       "5000000",
			RTSPFlags:     "prefer_tcp",
			RTSPTransport: "tcp",
		}

		// Формируем параметры видеокодирования, используя значения из конфигурации
		videoParams := &VideoEncodingParams{
			Codec:       VideoCodecH264,
			Preset:      PresetUltrafast,
			Tune:        TuneZerolatency,
			Profile:     ProfileBaseline,
			Level:       Level3_0,
			FrameRate:   c.cfg.FFmpeg.FrameRate,
			GOPSize:     c.cfg.FFmpeg.GOPSize,
			KeyIntMin:   c.cfg.FFmpeg.KeyIntMin,
			Bitrate:     c.cfg.FFmpeg.VideoBitrate,
			MaxRate:     c.cfg.FFmpeg.VideoMaxRate,
			MinRate:     c.cfg.FFmpeg.VideoMinRate,
			BufSize:     c.cfg.FFmpeg.VideoBufSize,
			PixelFormat: PixelFormatYUV420P,
			SceneChange: false,
			BFrames:     0,
			VSync:       "1",
			AvoidNegTS:  "1",
		}

		// Формируем параметры аудиокодирования (если есть аудио), используя значения из конфигурации
		var audioParams *AudioEncodingParams
		if streamInfo.HasAudio {
			audioParams = &AudioEncodingParams{
				Codec:      AudioCodecAAC,
				Bitrate:    c.cfg.FFmpeg.AudioBitrate,
				SampleRate: c.cfg.FFmpeg.AudioSampleRate,
			}
		}

		// Формируем HLS параметры, используя значения из конфигурации
		hlsSegmentPattern := fmt.Sprintf("%s/%s_segment_%%03d.ts", hlsDir, streamID)
		hlsParams := &HLSParams{
			HLSFormat:      HLSFormatMPEGTS,
			SegmentTime:    c.cfg.FFmpeg.HLSSegmentTime,
			HLSListSize:    c.cfg.FFmpeg.HLSListSize,
			HLSFlags:       "append_list+discont_start+split_by_time",
			SegmentPattern: hlsSegmentPattern,
			InitTime:       "0",
			MPEGTSFlags:    "+resend_headers",
			PATPeriod:      "0.1",
			SDTPeriod:      "0.1",
			PlaylistPath:   hlsPlaylist,
		}

		// Собираем все аргументы
		args := inputParams.ToArgs()
		args = append(args, videoParams.ToArgs()...)
		args = append(args, "-map", "0:v:0") // Маппинг видеопотока
		if streamInfo.HasAudio && audioParams != nil {
			args = append(args, audioParams.ToArgs()...)
		}
		args = append(args, hlsParams.ToArgs()...)

		ffmpegCmd := exec.Command("ffmpeg", args...)

		var stderr bytes.Buffer
		ffmpegCmd.Stderr = &stderr
		ffmpegCmd.Stdout = &stderr

		// Настраиваем StdinPipe до запуска процесса
		stdin, err := ffmpegCmd.StdinPipe()
		if err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to set up Stdin pipe for FFmpeg: %v", err))
			recordChan <- recordResult{err: fmt.Errorf("failed to set up Stdin pipe for FFmpeg: %w", err)}
			return
		}
		defer stdin.Close() // Закрываем Stdin после использования

		// Для отладки записываем вывод FFmpeg в файл
		f, err := os.Create(fmt.Sprintf("ffmpeg_output_%s.log", streamID))
		if err == nil {
			defer f.Close()
			mw := io.MultiWriter(f, &stderr)
			ffmpegCmd.Stderr = mw
			ffmpegCmd.Stdout = mw
		} else {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to create FFmpeg log file: %v", err))
		}

		// Логируем команду FFmpeg для отладки
		c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("FFmpeg command: ffmpeg %s", strings.Join(args, " ")))

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
			c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Received cancellation, sending 'q' to FFmpeg for stream %s", streamID))
			if ffmpegCmd.Process != nil {
				// Отправляем команду 'q' через уже настроенный Stdin
				if _, err := stdin.Write([]byte("q\n")); err != nil {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to send 'q' to FFmpeg: %v", err))
				}
			}

			// Даем FFmpeg больше времени на завершение
			select {
			case err := <-done:
				if err != nil {
					c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("FFmpeg exited with error after 'q': %v, FFmpeg output: %s", err, stderr.String()))
				} else {
					c.logger.Info("ProcessStream", "rtsp.go", "FFmpeg completed gracefully after 'q'")
				}
			case <-time.After(500 * time.Millisecond):
				c.logger.Warning("ProcessStream", "rtsp.go", "FFmpeg did not exit within 500 milliseconds, killing process")
				c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("FFmpeg output before killing: %s", stderr.String()))
				if ffmpegCmd.Process != nil {
					if err := ffmpegCmd.Process.Kill(); err != nil {
						c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to kill FFmpeg process: %v", err))
					}
				}
			}

			// Вычисляем продолжительность записи
			duration := int(time.Since(startTime).Seconds())
			recordChan <- recordResult{duration: duration, err: nil}
			return

		case err := <-done:
			// FFmpeg завершился сам
			duration := int(time.Since(startTime).Seconds())
			if err != nil {
				c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to record video with FFmpeg: %v, FFmpeg output: %s", err, stderr.String()))
				recordChan <- recordResult{err: fmt.Errorf("failed to record video: %w, FFmpeg output: %s", err, stderr.String())}
				return
			}
			recordChan <- recordResult{duration: duration, err: nil}
		}
	}()

	// Ожидаем результат записи
	var duration int
	var newCtx context.Context
	var cancel context.CancelFunc
	res := <-recordChan
	if res.err != nil {
		// Обновляем продолжительность в stream_metadata
		newCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		metaUpdate := &database.StreamMetadata{
			StreamID: streamID,
			Duration: duration,
		}
		if err := c.storage.UpdateStreamMetadata(newCtx, metaUpdate); err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to update stream metadata duration: %v", err))
		}
		return res.err
	}
	duration = res.duration
	// Создаем новый контекст для дальнейших операций
	newCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Логируем продолжение обработки
	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Proceeding with post-processing for streamID %s", streamID))

	// Обновляем продолжительность в stream_metadata
	metaUpdate := &database.StreamMetadata{
		StreamID: streamID,
		Duration: duration,
	}
	if err := c.storage.UpdateStreamMetadata(newCtx, metaUpdate); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to update stream metadata duration: %v", err))
		return fmt.Errorf("failed to update stream metadata duration: %w", err)
	}

	// Этап 2: Построение Merkle-дерева для HLS-сегментов
	go func() {
		c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Starting Merkle tree construction for HLS segments of streamID %s", streamID))
		blocks, tree, err := c.buildMerkleTreeForHLSSegments(hlsDir, streamID)
		merkleChan <- merkleResult{blocks: blocks, tree: tree, err: err}
	}()

	// Ожидаем результаты построения Merkle-дерева
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
	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("Preparing to save HLS Merkle proofs for streamID %s", streamID))

	// Проверяем подключение к базе данных
	if err := c.storage.Ping(newCtx); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Database connection failed: %v", err))
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Генерируем и сохраняем доказательства включения для HLS-сегментов
	for i := 0; i < len(blocks); i++ {
		proof, err := tree.GenerateProof(i)
		if err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to generate Merkle proof for segment %d: %v", i, err))
			continue
		}

		proofPath, err := json.Marshal(proof.Path)
		if err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to serialize Merkle proof for segment %d: %v", i, err))
			continue
		}

		merkleProof := &database.HLSMerkleProof{
			StreamID:     streamID,
			StreamName:   streamName,
			SegmentIndex: i,
			ProofPath:    string(proofPath),
			CreatedAt:    time.Now(),
		}
		if err := c.storage.SaveHLSMerkleProof(newCtx, merkleProof); err != nil {
			c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save HLS Merkle proof for segment %d: %v", i, err))
			continue
		}
	}

	// Сохраняем информацию о HLS в базе данных
	hlsPlaylistEntry := &database.HLSPlaylist{
		StreamID:     streamID,
		StreamName:   streamName,
		PlaylistPath: hlsPlaylist,
		CreatedAt:    time.Now(),
	}
	if err := c.storage.SaveHLSPlaylist(newCtx, hlsPlaylistEntry); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save HLS playlist: %v", err))
		return fmt.Errorf("failed to save HLS playlist: %w", err)
	}
	c.logger.Info("ProcessStream", "rtsp.go", fmt.Sprintf("HLS generated at %s for streamID %s", hlsPlaylist, streamID))

	// Сохраняем информацию о завершённом стриме в таблицу archive
	archiveEntry := &database.Archive{
		StreamID:        streamID,
		StreamName:      streamName,
		Status:          "completed",
		Duration:        duration,
		HLSPlaylistPath: hlsPlaylist,
		ArchivedAt:      time.Now(),
	}
	if err := c.storage.ArchiveStream(newCtx, archiveEntry); err != nil {
		c.logger.Error("ProcessStream", "rtsp.go", fmt.Sprintf("Failed to save archive entry: %v", err))
		return fmt.Errorf("failed to save archive entry: %w", err)
	}

	// Логируем успешное завершение
	logEntry = &database.ProcessingLog{
		StreamID:   streamID,
		StreamName: streamName,
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

// buildMerkleTreeForHLSSegments строит Merkle-дерево на основе HLS-сегментов
func (c *RTSPClient) buildMerkleTreeForHLSSegments(hlsDir, streamID string) ([][]byte, *merkle.MerkleTree, error) {
	// Читаем все HLS-сегменты из директории
	pattern := filepath.Join(hlsDir, fmt.Sprintf("%s_segment_*.ts", streamID))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list HLS segments: %w", err)
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no HLS segments found in %s", hlsDir)
	}

	// Сортируем файлы по имени, чтобы сегменты шли по порядку
	sort.Strings(files)

	// Создаём блоки для Merkle-дерева (хэши сегментов)
	var blocks [][]byte
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			c.logger.Error("buildMerkleTreeForHLSSegments", "rtsp.go", fmt.Sprintf("Failed to read HLS segment %s: %v", file, err))
			continue
		}
		hash := sha256.Sum256(data)
		blocks = append(blocks, hash[:])
	}

	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("no valid HLS segments to build Merkle tree")
	}

	// Строим Merkle-дерево
	tree, err := merkle.NewMerkleTree(blocks)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build Merkle tree: %w", err)
	}

	return blocks, tree, nil
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
