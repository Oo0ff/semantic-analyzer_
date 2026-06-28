package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Transcript represents a text transcript (from text file or audio transcription) with metadata
type Transcript struct {
	ID                     string                 `json:"id"`
	RawText                string                 `json:"raw_text"`                  // Final text used for analysis (transcribed or original)
	ProcessedText          string                 `json:"processed_text,omitempty"`
	Sentences              []string               `json:"sentences,omitempty"`
	TokenCount             int                    `json:"token_count"`
	Source                 string                 `json:"source"`                    // filename or "audio:meeting.wav"
	CreatedAt              time.Time              `json:"created_at"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	Encoding               string                 `json:"encoding,omitempty"`
	Language               string                 `json:"language,omitempty"`
	CharacterCount         int                    `json:"character_count"`
	LineCount              int                    `json:"line_count,omitempty"`

	// ────────────────────────────────────────────────
	// New fields specifically for audio transcription
	// ────────────────────────────────────────────────
	IsTranscribedFromAudio   bool     `json:"is_transcribed_from_audio"`   // true if text came from STT
	AudioDurationSeconds     float64  `json:"audio_duration_seconds,omitempty"`
	AudioSampleRate          int      `json:"audio_sample_rate,omitempty"` // e.g. 16000 Hz
	AudioSourceFormat        string   `json:"audio_source_format,omitempty"` // "wav", "mp3", etc.
	TranscriptionModel       string   `json:"transcription_model,omitempty"` // e.g. "vosk-small-en-us-0.15"
	TranscriptionConfidence  float64  `json:"transcription_confidence,omitempty"` // if model provides average confidence
	TranscriptionDurationMs  int64    `json:"transcription_duration_ms,omitempty"` // how long transcription took
}

// NewTranscript creates a new transcript
// For audio: pass audio metadata via opts map (optional)
func NewTranscript(rawText, source string, opts ...map[string]interface{}) *Transcript {
	t := &Transcript{
		ID:                     "", // Will be set by hash later
		RawText:                rawText,
		Source:                 source,
		CreatedAt:              time.Now(),
		Metadata:               make(map[string]interface{}),
		Encoding:               "UTF-8",
		Language:               "en", // can be updated later
		CharacterCount:         len([]rune(rawText)),
		IsTranscribedFromAudio: false,
	}

	// Apply optional audio-related parameters
	if len(opts) > 0 {
		opt := opts[0]
		if v, ok := opt["is_audio"]; ok {
			if b, ok := v.(bool); ok {
				t.IsTranscribedFromAudio = b
			}
		}
		if v, ok := opt["duration_sec"]; ok {
			if f, ok := v.(float64); ok {
				t.AudioDurationSeconds = f
			}
		}
		if v, ok := opt["sample_rate"]; ok {
			if i, ok := v.(int); ok {
				t.AudioSampleRate = i
			}
		}
		if v, ok := opt["format"]; ok {
			if s, ok := v.(string); ok {
				t.AudioSourceFormat = s
			}
		}
		if v, ok := opt["model"]; ok {
			if s, ok := v.(string); ok {
				t.TranscriptionModel = s
			}
		}
	}

	return t
}

// Validate checks basic constraints
func (t *Transcript) Validate() error {
	if t.RawText == "" {
		return &ValidationError{Field: "RawText", Reason: "cannot be empty"}
	}

	maxSize := 1000000 // 1M characters
	if t.CharacterCount > maxSize {
		return &ValidationError{
			Field:  "RawText",
			Reason: fmt.Sprintf("exceeds maximum size (%d characters)", maxSize),
		}
	}

	if t.IsTranscribedFromAudio {
		if t.AudioSampleRate <= 0 {
			return &ValidationError{Field: "AudioSampleRate", Reason: "must be positive when audio transcription is used"}
		}
		if t.AudioDurationSeconds < 0 {
			return &ValidationError{Field: "AudioDurationSeconds", Reason: "cannot be negative"}
		}
	}

	return nil
}

// GetSentence returns a sentence with bounds checking
func (t *Transcript) GetSentence(index int) (string, error) {
	if index < 0 || index >= len(t.Sentences) {
		return "", &IndexError{Index: index, Length: len(t.Sentences)}
	}
	return t.Sentences[index], nil
}

// GetTextSpan returns substring between positions (on ProcessedText)
func (t *Transcript) GetTextSpan(start, end int) (string, error) {
	runes := []rune(t.ProcessedText)
	if start < 0 || end > len(runes) || start > end {
		return "", &SpanError{Start: start, End: end, Length: len(runes)}
	}
	return string(runes[start:end]), nil
}

// ToJSON serializes to pretty JSON
func (t *Transcript) ToJSON() ([]byte, error) {
	return json.MarshalIndent(t, "", "  ")
}

// FromJSON deserializes from JSON
func (t *Transcript) FromJSON(data []byte) error {
	return json.Unmarshal(data, t)
}

// AddMetadata safely adds a key-value pair
func (t *Transcript) AddMetadata(key string, value interface{}) {
	if t.Metadata == nil {
		t.Metadata = make(map[string]interface{})
	}
	t.Metadata[key] = value
}

// GetMetadata retrieves value by key
func (t *Transcript) GetMetadata(key string) (interface{}, bool) {
	if t.Metadata == nil {
		return nil, false
	}
	value, exists := t.Metadata[key]
	return value, exists
}

// ────────────────────────────────────────────────
// Error types (unchanged)
// ────────────────────────────────────────────────

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field + " - " + e.Reason
}

type IndexError struct {
	Index  int
	Length int
}

func (e *IndexError) Error() string {
	return fmt.Sprintf("index error: index %d out of bounds (length %d)", e.Index, e.Length)
}

type SpanError struct {
	Start  int
	End    int
	Length int
}

func (e *SpanError) Error() string {
	return fmt.Sprintf("span error: start %d, end %d (length %d)", e.Start, e.End, e.Length)
}