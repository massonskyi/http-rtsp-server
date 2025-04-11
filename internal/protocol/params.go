package protocol

import "fmt"

// FFmpeg перечисления для фиксированных значений
type VideoCodec string

const (
	VideoCodecH264 VideoCodec = "libx264"
)

type Preset string

const (
	PresetUltrafast Preset = "ultrafast"
	PresetSuperfast Preset = "superfast"
	PresetVeryfast  Preset = "veryfast"
	PresetFaster    Preset = "faster"
	PresetFast      Preset = "fast"
	PresetMedium    Preset = "medium"
	PresetSlow      Preset = "slow"
	PresetSlower    Preset = "slower"
	PresetVeryslow  Preset = "veryslow"
)

type Tune string

const (
	TuneZerolatency Tune = "zerolatency"
)

type Profile string

const (
	ProfileBaseline Profile = "baseline"
	ProfileMain     Profile = "main"
	ProfileHigh     Profile = "high"
)

type Level string

const (
	Level3_0 Level = "3.0"
	Level4_0 Level = "4.0"
	Level4_1 Level = "4.1"
)

type PixelFormat string

const (
	PixelFormatYUV420P PixelFormat = "yuv420p"
)

type AudioCodec string

const (
	AudioCodecAAC AudioCodec = "aac"
)

type HLSFormat string

const (
	HLSFormatMPEGTS HLSFormat = "mpegts"
)

// InputParams содержит входные параметры для FFmpeg
type InputParams struct {
	RTSPURL       string
	BufferSize    string
	Timeout       string
	RTSPFlags     string
	RTSPTransport string
}

// ToArgs возвращает входные параметры в виде слайса аргументов
func (p *InputParams) ToArgs() []string {
	return []string{
		"-fflags", "+genpts+discardcorrupt",
		"-use_wallclock_as_timestamps", "1",
		"-rtsp_transport", p.RTSPTransport,
		"-buffer_size", p.BufferSize,
		"-rtsp_flags", p.RTSPFlags,
		"-timeout", p.Timeout,
		"-i", p.RTSPURL,
	}
}

// VideoEncodingParams содержит параметры видеокодирования
type VideoEncodingParams struct {
	Codec       VideoCodec
	Preset      Preset
	Tune        Tune
	Profile     Profile
	Level       Level
	FrameRate   string
	GOPSize     int
	KeyIntMin   int
	Bitrate     string
	MaxRate     string
	MinRate     string
	BufSize     string
	PixelFormat PixelFormat
	SceneChange bool
	BFrames     int
	VSync       string
	AvoidNegTS  string
}

// ToArgs возвращает параметры видеокодирования в виде слайса аргументов
func (p *VideoEncodingParams) ToArgs() []string {
	args := []string{
		"-c:v", string(p.Codec),
		"-preset", string(p.Preset),
		"-tune", string(p.Tune),
		"-profile:v", string(p.Profile),
		"-level", string(p.Level),
		"-r", p.FrameRate,
		"-g", fmt.Sprintf("%d", p.GOPSize),
		"-keyint_min", fmt.Sprintf("%d", p.KeyIntMin),
		"-b:v", p.Bitrate,
		"-maxrate", p.MaxRate,
		"-minrate", p.MinRate,
		"-bufsize", p.BufSize,
		"-pix_fmt", string(p.PixelFormat),
		"-vsync", p.VSync,
		"-avoid_negative_ts", p.AvoidNegTS,
	}

	// Формируем x264 параметры
	x264Params := fmt.Sprintf("no-scenecut=%d:bframes=%d", boolToInt(!p.SceneChange), p.BFrames)
	args = append(args, "-x264-params", x264Params)

	// Добавляем отключение смены сцен
	if !p.SceneChange {
		args = append(args, "-sc_threshold", "0")
	}

	return args
}

// AudioEncodingParams содержит параметры аудиокодирования
type AudioEncodingParams struct {
	Codec      AudioCodec
	Bitrate    string
	SampleRate string
}

// ToArgs возвращает параметры аудиокодирования в виде слайса аргументов
func (p *AudioEncodingParams) ToArgs() []string {
	return []string{
		"-map", "0:a:0",
		"-c:a", string(p.Codec),
		"-b:a", p.Bitrate,
		"-ar", p.SampleRate,
	}
}

// HLSParams содержит параметры для HLS-формата
type HLSParams struct {
	HLSFormat      HLSFormat
	SegmentTime    string
	HLSListSize    string
	HLSFlags       string
	SegmentPattern string
	InitTime       string
	MPEGTSFlags    string
	PATPeriod      string
	SDTPeriod      string
	PlaylistPath   string
}

// ToArgs возвращает параметры HLS в виде слайса аргументов
func (p *HLSParams) ToArgs() []string {
	return []string{
		"-f", "hls",
		"-hls_time", p.SegmentTime,
		"-hls_list_size", p.HLSListSize,
		"-hls_flags", p.HLSFlags,
		"-hls_segment_type", string(p.HLSFormat),
		"-hls_segment_filename", p.SegmentPattern,
		"-hls_init_time", p.InitTime,
		"-mpegts_flags", p.MPEGTSFlags,
		"-pat_period", p.PATPeriod,
		"-sdt_period", p.SDTPeriod,
		p.PlaylistPath,
	}
}

// boolToInt конвертирует bool в int (0 или 1)
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
