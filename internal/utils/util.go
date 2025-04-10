package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// StreamInfo содержит информацию о потоке, полученную через ffprobe
type StreamInfo struct {
	HasAudio bool
	Width    int
	Height   int
}

// ProbeStream проверяет RTSP-поток с помощью ffprobe и возвращает информацию о нём
func ProbeStream(rtspURL string) (*StreamInfo, error) {
	// Формируем команду ffprobe
	args := []string{
		"-v", "error", // Минимизируем вывод логов
		"-show_streams",          // Показываем информацию о потоках
		"-select_streams", "v:0", // Выбираем первый видеопоток
		"-show_entries", "stream=width,height", // Извлекаем ширину и высоту
		"-of", "json", // Формат вывода - JSON
		"-rtsp_transport", "tcp", // Используем TCP для RTSP
		"-i", rtspURL,
	}

	// Запускаем ffprobe
	cmd := exec.Command("ffprobe", args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %v, stderr: %s", err, stderr.String())
	}

	// Парсим JSON-вывод ffprobe
	var probeOutput struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out.Bytes(), &probeOutput); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %v", err)
	}

	// Проверяем, есть ли видеопоток
	if len(probeOutput.Streams) == 0 {
		return nil, fmt.Errorf("no video stream found in RTSP URL: %s", rtspURL)
	}

	// Извлекаем ширину и высоту
	videoStream := probeOutput.Streams[0]
	streamInfo := &StreamInfo{
		Width:  videoStream.Width,
		Height: videoStream.Height,
	}

	// Проверяем наличие аудиопотока
	args = []string{
		"-v", "error",
		"-show_streams",
		"-select_streams", "a:0", // Выбираем первый аудиопоток
		"-of", "json",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
	}

	cmd = exec.Command("ffprobe", args...)
	out.Reset()
	stderr.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		// Если аудиопоток не найден, это не ошибка, просто логируем
		fmt.Printf("No audio stream found: %v, stderr: %s\n", err, stderr.String())
	} else {
		var audioProbe struct {
			Streams []struct{} `json:"streams"`
		}
		if err := json.Unmarshal(out.Bytes(), &audioProbe); err != nil {
			return nil, fmt.Errorf("failed to parse ffprobe audio output: %v", err)
		}
		if len(audioProbe.Streams) > 0 {
			streamInfo.HasAudio = true
		}
	}

	return streamInfo, nil
}
