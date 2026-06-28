package domain

import (
    "encoding/json"
    "fmt"
    "time"
    "strings"
)

// Marker represents a detected pattern in text
type Marker struct {
    ID                string                 `json:"id"`
    Level             int                    `json:"level"`
    Type              string                 `json:"type"`
    TextSpan          string                 `json:"text_span"`
    Start             int                    `json:"start"`
    End               int                    `json:"end"`
    Confidence        int64                  `json:"confidence"`           // fixed-point
    RuleID            string                 `json:"rule_id"`
    Context           string                 `json:"context,omitempty"`
    SentenceID        int                    `json:"sentence_id,omitempty"`
    DetectedAt        time.Time              `json:"detected_at"`
    Metadata          map[string]interface{} `json:"metadata,omitempty"`
    ParentID          string                 `json:"parent_id,omitempty"`
    Children          []string               `json:"children,omitempty"`
    IsAtomic          bool                   `json:"is_atomic"`
    Score             int64                  `json:"score,omitempty"`      // fixed-point
    Weight            int64                  `json:"weight,omitempty"`     // fixed-point
    TraceID           string                 `json:"trace_id"`
    EventIDs          []string               `json:"event_ids,omitempty"`
    ConfigVersion     string                 `json:"config_version"`
    QualityBucket     string                 `json:"quality_bucket,omitempty"`
    HumanInLoop       bool                   `json:"human_in_loop"`
    HilReason         string                 `json:"hil_reason,omitempty"`
    NegativeHit       bool                   `json:"negative_hit,omitempty"`
    SuppressionReason string                 `json:"suppression_reason,omitempty"`
    Serial            int                    `json:"serial,omitempty"`
}

// NewMarker creates a new marker with fixed-point confidence and provenance fields
func NewMarker(level int, markerType, textSpan string, start, end int,
    confidence int64, ruleID, traceID, configVersion string, isAtomic bool) *Marker {

    return &Marker{
        ID:            fmt.Sprintf("marker_%d_%d_%s", start, end, ruleID),
        Level:         level,
        Type:          markerType,
        TextSpan:      textSpan,
        Start:         start,
        End:           end,
        Confidence:    confidence,
        RuleID:        ruleID,
        DetectedAt:    time.Now(),
        Metadata:      make(map[string]interface{}),
        IsAtomic:      isAtomic,
        Score:         confidence, // default score equals confidence
        Weight:        1000000,     // default weight = 1.0 in fixed-point (assuming FixedPointScale = 1000000)
        TraceID:       traceID,
        ConfigVersion: configVersion,
        HumanInLoop:   false,
        NegativeHit:   false,
    }
}

// Validate checks if the marker is valid
func (m *Marker) Validate() error {
    if m.Start < 0 || m.End < 0 {
        return &MarkerValidationError{
            Field:  "Start/End",
            Reason: "cannot be negative",
        }
    }

    if m.Start > m.End {
        return &MarkerValidationError{
            Field:  "Start/End",
            Reason: "start must be <= end",
        }
    }

    // Confidence is fixed-point; we skip range check here or could check >=0
    if m.Confidence < 0 {
        return &MarkerValidationError{
            Field:  "Confidence",
            Reason: "cannot be negative",
        }
    }

    if m.Level < 1 || m.Level > 5 {
        return &MarkerValidationError{
            Field:  "Level",
            Reason: "must be between 1 and 5",
        }
    }

    if m.Type == "" {
        return &MarkerValidationError{
            Field:  "Type",
            Reason: "cannot be empty",
        }
    }

    if m.TextSpan == "" {
        return &MarkerValidationError{
            Field:  "TextSpan",
            Reason: "cannot be empty",
        }
    }

    return nil
}

// Overlaps checks if this marker overlaps with another marker
func (m *Marker) Overlaps(other *Marker) bool {
    return !(m.End <= other.Start || m.Start >= other.End)
}

// Contains checks if this marker contains another marker
func (m *Marker) Contains(other *Marker) bool {
    return m.Start <= other.Start && m.End >= other.End
}

// DistanceTo calculates the distance to another marker
func (m *Marker) DistanceTo(other *Marker) int {
    if m.Overlaps(other) {
        return 0
    }
    if m.End <= other.Start {
        return other.Start - m.End
    }
    return m.Start - other.End
}

// AddMetadata adds metadata to the marker
func (m *Marker) AddMetadata(key string, value interface{}) {
    if m.Metadata == nil {
        m.Metadata = make(map[string]interface{})
    }
    m.Metadata[key] = value
}

// GetMetadata retrieves metadata by key
func (m *Marker) GetMetadata(key string) (interface{}, bool) {
    if m.Metadata == nil {
        return nil, false
    }
    value, exists := m.Metadata[key]
    return value, exists
}

// SetContext sets the context around the marker (byte‑based indices)
func (m *Marker) SetContext(fullText string, contextSize int) {
    // Защита от некорректных позиций
    if m.Start < 0 {
        m.Start = 0
    }
    if m.End > len(fullText) {
        m.End = len(fullText)
    }
    if m.Start > m.End {
        m.Start = m.End
    }

    // Вычисляем границы контекста в байтах
    start := max(0, m.Start-contextSize)
    end := min(len(fullText), m.End+contextSize)

    var sb strings.Builder
    if start < m.Start {
        sb.WriteString("...")
        sb.WriteString(fullText[start:m.Start])
    } else {
        sb.WriteString(fullText[start:m.Start])
    }
    sb.WriteString(fullText[m.Start:m.End])
    if end > m.End {
        sb.WriteString(fullText[m.End:end])
        sb.WriteString("...")
    }
    m.Context = sb.String()
}

// ToJSON serializes the marker to JSON
func (m *Marker) ToJSON() ([]byte, error) {
    return json.MarshalIndent(m, "", "  ")
}

// FromJSON deserializes JSON into a marker
func (m *Marker) FromJSON(data []byte) error {
    return json.Unmarshal(data, m)
}

// Helper functions
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}

// Error type for marker validation
type MarkerValidationError struct {
    Field  string
    Reason string
}

func (e *MarkerValidationError) Error() string {
    return fmt.Sprintf("marker validation error: %s - %s", e.Field, e.Reason)
}