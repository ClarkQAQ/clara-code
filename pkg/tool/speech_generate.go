package tool

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	clarafs "clara-code/pkg/tool/fs"
)

// Audio is generated speech data.
type Audio struct {
	Data       []byte
	MIME       string
	SampleRate int
	Channels   int
}

// SpeechRequest is one synthesis request.
type SpeechRequest struct {
	Text   string
	Voice  string
	Format string
}

// Provider synthesizes speech.
type Provider interface {
	Voices() []string
	Synthesize(context.Context, SpeechRequest) (Audio, error)
}

// SpeechHTTPDoer is the HTTP client contract.
type SpeechHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// BreezeConfig configures a Breeze-compatible PCM speech endpoint.
type BreezeConfig struct {
	BaseURL    string
	APIKey     string
	Voices     map[string]string
	Default    string
	HTTPClient SpeechHTTPDoer
}

// Breeze returns PCM16 speech and wraps it as WAV.
type Breeze struct {
	baseURL      string
	apiKey       string
	voices       map[string]string
	names        []string
	defaultVoice string
	client       SpeechHTTPDoer
}

// NewBreeze creates a Breeze provider.
func NewBreeze(config BreezeConfig) (*Breeze, error) {
	base := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if base == "" {
		return nil, errors.New("voice: Breeze base URL is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 5 * time.Minute}
	}
	if len(config.Voices) == 0 {
		return nil, errors.New("voice: at least one Breeze voice mapping is required")
	}
	names := make([]string, 0, len(config.Voices))
	for name := range config.Voices {
		names = append(names, name)
	}
	sort.Strings(names)
	defaultVoice := config.Default
	if defaultVoice == "" {
		defaultVoice = names[0]
	}
	if _, ok := config.Voices[defaultVoice]; !ok {
		return nil, fmt.Errorf("voice: default voice %q is not mapped", defaultVoice)
	}
	return &Breeze{
		baseURL: base, apiKey: config.APIKey, voices: config.Voices,
		names: names, defaultVoice: defaultVoice, client: config.HTTPClient,
	}, nil
}

func (b *Breeze) Voices() []string { return append([]string(nil), b.names...) }

func (b *Breeze) Synthesize(ctx context.Context, request SpeechRequest) (Audio, error) {
	voice := request.Voice
	if voice == "" {
		voice = b.defaultVoice
	}
	target, ok := b.voices[voice]
	if !ok {
		return Audio{}, fmt.Errorf("voice: unknown voice %q", voice)
	}
	form := url.Values{"text": {request.Text}, "voice_id": {target}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/audio/speech",
		strings.NewReader(form.Encode()))
	if err != nil {
		return Audio{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if b.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.apiKey)
	}
	response, err := b.client.Do(req)
	if err != nil {
		return Audio{}, err
	}
	defer response.Body.Close()
	pcm, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return Audio{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Audio{}, fmt.Errorf("voice: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(pcm)))
	}
	sampleRate := 24000
	if raw := response.Header.Get("X-Sample-Rate"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &sampleRate); err != nil {
			sampleRate = 24000
		}
	}
	wav, err := PCM16WAV(pcm, sampleRate, 1)
	if err != nil {
		return Audio{}, err
	}
	return Audio{Data: wav, MIME: "audio/wav", SampleRate: sampleRate, Channels: 1}, nil
}

// OpenAIConfig configures an OpenAI-compatible speech endpoint.
type OpenAIConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Voices     []string
	Default    string
	HTTPClient SpeechHTTPDoer
}

// OpenAI calls an OpenAI-compatible /v1/audio/speech endpoint.
type OpenAI struct {
	baseURL      string
	apiKey       string
	model        string
	voices       []string
	defaultVoice string
	client       SpeechHTTPDoer
}

func NewOpenAI(config OpenAIConfig) (*OpenAI, error) {
	base := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if base == "" {
		return nil, errors.New("voice: OpenAI-compatible base URL is required")
	}
	if config.Model == "" {
		config.Model = "tts-1"
	}
	if len(config.Voices) == 0 {
		config.Voices = []string{"alloy"}
	}
	if config.Default == "" {
		config.Default = config.Voices[0]
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 5 * time.Minute}
	}
	return &OpenAI{
		baseURL: base, apiKey: config.APIKey, model: config.Model,
		voices: append([]string(nil), config.Voices...), defaultVoice: config.Default,
		client: config.HTTPClient,
	}, nil
}

func (o *OpenAI) Voices() []string { return append([]string(nil), o.voices...) }

func (o *OpenAI) Synthesize(ctx context.Context, request SpeechRequest) (Audio, error) {
	voice := request.Voice
	if voice == "" {
		voice = o.defaultVoice
	}
	body, _ := json.Marshal(map[string]string{
		"model": o.model, "input": request.Text, "voice": voice, "response_format": "wav",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return Audio{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	response, err := o.client.Do(req)
	if err != nil {
		return Audio{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return Audio{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Audio{}, fmt.Errorf("voice: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if !bytes.HasPrefix(data, []byte("RIFF")) {
		return Audio{}, errors.New("voice: provider did not return WAV audio")
	}
	return Audio{Data: data, MIME: "audio/wav", SampleRate: 24000, Channels: 1}, nil
}

// Speech writes generated speech into an FS.
type Speech struct {
	FS       clarafs.FS
	Provider Provider
}

// New creates a voice tool set.
func NewSpeech(storage clarafs.FS, provider Provider) *Speech {
	return &Speech{FS: storage, Provider: provider}
}

// All returns speech_generate.
func (s *Speech) All() []Tool {
	if s == nil || s.Provider == nil {
		return nil
	}
	return []Tool{s.Generate()}
}

func (s *Speech) Generate() Tool {
	type input struct {
		Text  string `json:"text" description:"text to speak"`
		Path  string `json:"path" description:"output .wav path"`
		Voice string `json:"voice,omitempty" description:"configured voice name"`
	}
	return Func(Definition{
		Name: "speech_generate",
		Description: "Generate a WAV voice file from text using a configured speech provider. " +
			"Returns the path, duration estimate, and sample rate.",
		Effects: []Effect{EffectNetwork, EffectWrite, EffectAudio},
		Timeout: 5 * time.Minute,
	}, func(ctx context.Context, in input) (Result, error) {
		if strings.TrimSpace(in.Text) == "" {
			return ErrorResult("speech text is required"), nil
		}
		target, err := clarafs.ResolvePath(s.FS, in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		if strings.ToLower(path.Ext(target.String())) != ".wav" {
			return ErrorResult("speech output must end in .wav"), nil
		}
		audio, err := s.Provider.Synthesize(ctx, SpeechRequest{Text: in.Text, Voice: in.Voice, Format: "wav"})
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		entry, err := s.FS.WriteFile(ctx, target, audio.Data, clarafs.WriteOptions{
			Mode: 0o644, CreateDirs: true,
		})
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		duration := 0.0
		if audio.SampleRate > 0 && audio.Channels > 0 {
			duration = float64(max(0, len(audio.Data)-44)) / float64(audio.SampleRate*audio.Channels*2)
		}
		payload, _ := json.Marshal(map[string]any{
			"path": entry.Path, "mime": audio.MIME, "bytes": entry.Size,
			"duration_seconds": duration, "sample_rate": audio.SampleRate,
		})
		return MediaResult(string(payload), audio.Data, audio.MIME), nil
	})
}

// PCM16WAV wraps little-endian mono PCM in a WAV container.
func PCM16WAV(pcm []byte, sampleRate, channels int) ([]byte, error) {
	if sampleRate <= 0 || channels <= 0 {
		return nil, errors.New("voice: invalid WAV format")
	}
	if len(pcm)%2 != 0 {
		pcm = append(pcm, 0)
	}
	const headerSize = 44
	dataSize := uint32(len(pcm))
	blockAlign := uint16(channels * 2)
	byteRate := uint32(sampleRate * channels * 2)
	var buffer bytes.Buffer
	buffer.WriteString("RIFF")
	_ = binary.Write(&buffer, binary.LittleEndian, 36+dataSize)
	buffer.WriteString("WAVEfmt ")
	_ = binary.Write(&buffer, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&buffer, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buffer, binary.LittleEndian, byteRate)
	_ = binary.Write(&buffer, binary.LittleEndian, blockAlign)
	_ = binary.Write(&buffer, binary.LittleEndian, uint16(16))
	buffer.WriteString("data")
	_ = binary.Write(&buffer, binary.LittleEndian, dataSize)
	buffer.Write(pcm)
	if buffer.Len() != headerSize+len(pcm) {
		return nil, errors.New("voice: WAV encoding failed")
	}
	return buffer.Bytes(), nil
}
