package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// AnalysisResult represents the final output of the analysis
type AnalysisResult struct {
	TranscriptID   string                 `json:"transcript_id"`
	Transcript     *Transcript            `json:"transcript,omitempty"`
	Markers        []Marker               `json:"markers"`
	AuditEvents    []AuditEvent           `json:"audit_events,omitempty"`
	AuditTrail     *AuditTrail            `json:"audit_trail,omitempty"`
	Statistics     Statistics             `json:"statistics"`
	Timestamp      time.Time              `json:"timestamp"`
	ConfigHash     string                 `json:"config_hash"`
	ProcessingTime float64                `json:"processing_time_seconds"`
	Version        string                 `json:"version"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Summary        string                 `json:"summary,omitempty"`  // экстрактивная сводка
	Topics         []TopicResult          `json:"topics"`   // тематическая классификация
}

// TopicResult – результат тематической классификации
type TopicResult struct {
	Code       string  `json:"code"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
}

// Statistics contains analysis metrics
type Statistics struct {
	TotalMarkers      int            `json:"total_markers"`
	AtomicMarkers     int            `json:"atomic_markers"`
	CompositeMarkers  int            `json:"composite_markers"`
	MarkerTypes       map[string]int `json:"marker_types"`
	ProcessingTime    float64        `json:"processing_time_seconds"`
	MemoryUsage       uint64         `json:"memory_usage_bytes,omitempty"`
	SentenceCount     int            `json:"sentence_count"`
	WordCount         int            `json:"word_count"`

	// Средняя уверенность в fixed-point (например, 650000 = 0.65)
	AverageConfidenceFixed int64 `json:"average_confidence_fixed,omitempty"`
	// Для удобства вывода — float64 в диапазоне [0,1]
	AverageConfidence float64 `json:"average_confidence"`
}

// NewAnalysisResult creates a new analysis result
func NewAnalysisResult(transcriptID string) *AnalysisResult {
	return &AnalysisResult{
		TranscriptID: transcriptID,
		Markers:      []Marker{},
		AuditEvents:  []AuditEvent{},
		Statistics: Statistics{
			MarkerTypes: make(map[string]int),
		},
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
		Version:   "1.0.0",
	}
}

// AddMarker adds a marker to the result
func (ar *AnalysisResult) AddMarker(marker Marker) error {
	if err := marker.Validate(); err != nil {
		return fmt.Errorf("invalid marker: %w", err)
	}

	ar.Markers = append(ar.Markers, marker)
	ar.Statistics.TotalMarkers++

	if marker.IsAtomic {
		ar.Statistics.AtomicMarkers++
	} else {
		ar.Statistics.CompositeMarkers++
	}

	// Update type counts
	ar.Statistics.MarkerTypes[marker.Type]++

	// Update average confidence
	ar.updateAverageConfidence()

	return nil
}

// AddAuditEvent adds an audit event to the result
func (ar *AnalysisResult) AddAuditEvent(event AuditEvent) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid audit event: %w", err)
	}

	ar.AuditEvents = append(ar.AuditEvents, event)
	return nil
}

// GetMarkersByType returns markers of a specific type
func (ar *AnalysisResult) GetMarkersByType(markerType string) []Marker {
	var result []Marker
	for _, marker := range ar.Markers {
		if marker.Type == markerType {
			result = append(result, marker)
		}
	}
	return result
}

// GetMarkersByLevel returns markers of a specific level
func (ar *AnalysisResult) GetMarkersByLevel(level int) []Marker {
	var result []Marker
	for _, marker := range ar.Markers {
		if marker.Level == level {
			result = append(result, marker)
		}
	}
	return result
}

// GetTopMarkers returns the top N markers by confidence (descending)
func (ar *AnalysisResult) GetTopMarkers(n int) []Marker {
	// Create a copy to avoid modifying the original slice
	markers := make([]Marker, len(ar.Markers))
	copy(markers, ar.Markers)

	// Sort by confidence (descending) – now both int64, so ok
	sort.Slice(markers, func(i, j int) bool {
		return markers[i].Confidence > markers[j].Confidence
	})

	// Return top N
	if n > len(markers) {
		n = len(markers)
	}
	return markers[:n]
}

// FilterMarkers filters markers based on a predicate
func (ar *AnalysisResult) FilterMarkers(predicate func(Marker) bool) []Marker {
	var result []Marker
	for _, marker := range ar.Markers {
		if predicate(marker) {
			result = append(result, marker)
		}
	}
	return result
}

// ToJSON serializes the analysis result to JSON
func (ar *AnalysisResult) ToJSON() ([]byte, error) {
	return json.MarshalIndent(ar, "", "  ")
}

// FromJSON deserializes JSON into an analysis result
func (ar *AnalysisResult) FromJSON(data []byte) error {
	return json.Unmarshal(data, ar)
}

// Validate checks if the analysis result is valid
func (ar *AnalysisResult) Validate() error {
	if ar.TranscriptID == "" {
		return &ResultValidationError{Field: "TranscriptID", Reason: "cannot be empty"}
	}

	if ar.Timestamp.IsZero() {
		return &ResultValidationError{Field: "Timestamp", Reason: "must be set"}
	}

	// Validate all markers
	for i, marker := range ar.Markers {
		if err := marker.Validate(); err != nil {
			return fmt.Errorf("marker %d invalid: %w", i, err)
		}
	}

	// Validate all audit events
	for i, event := range ar.AuditEvents {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("audit event %d invalid: %w", i, err)
		}
	}

	return nil
}

// AddMetadata adds metadata to the result
func (ar *AnalysisResult) AddMetadata(key string, value interface{}) {
	if ar.Metadata == nil {
		ar.Metadata = make(map[string]interface{})
	}
	ar.Metadata[key] = value
}

// GetMetadata retrieves metadata by key
func (ar *AnalysisResult) GetMetadata(key string) (interface{}, bool) {
	if ar.Metadata == nil {
		return nil, false
	}
	value, exists := ar.Metadata[key]
	return value, exists
}

// updateAverageConfidence обновляет среднюю уверенность (fixed-point и float)
func (ar *AnalysisResult) updateAverageConfidence() {
	if len(ar.Markers) == 0 {
		ar.Statistics.AverageConfidenceFixed = 0
		ar.Statistics.AverageConfidence = 0.0
		return
	}

	var total int64
	for _, marker := range ar.Markers {
		total += marker.Confidence
	}

	count := int64(len(ar.Markers))
	ar.Statistics.AverageConfidenceFixed = total / count
	ar.Statistics.AverageConfidence = float64(total) / float64(count*FixedPointScale)
}

// Error type for result validation
type ResultValidationError struct {
	Field  string
	Reason string
}

func (e *ResultValidationError) Error() string {
	return fmt.Sprintf("result validation error: %s - %s", e.Field, e.Reason)
}