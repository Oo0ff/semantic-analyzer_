package composite

import (
	"semantic-analyzer/internal/domain"
)

// FixedPointScale — глобальный масштаб (должен быть определён только один раз, в domain)
const FixedPointScale = 1000000

// CompositeCandidate represents a candidate for composite marker formation
type CompositeCandidate struct {
	ID            string                 `json:"id"`
	Markers       []domain.Marker        `json:"markers"`
	Score         int64                  `json:"score"`         // fixed-point (0..1_000_000)
	SpanStart     int                    `json:"span_start"`
	SpanEnd       int                    `json:"span_end"`
	SentenceIDs   []int                  `json:"sentence_ids"`
	Density       float64                `json:"density"`       // оставляем float64, т.к. это метрика, а не score
	TypeDiversity float64                `json:"type_diversity"`
	SpanLength    int                    `json:"span_length"`
	MarkerCount   int                    `json:"marker_count"`
	PrimaryType   string                 `json:"primary_type"`
	Confidence    int64                  `json:"confidence"`    // fixed-point
	Context       string                 `json:"context,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`

	// Новые поля по требованиям задачи
	QualityBucket string                 `json:"quality_bucket,omitempty"` // high/medium/low
	HumanInLoop   bool                   `json:"human_in_loop"`
	HilReason     string                 `json:"hil_reason,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
}

// NewCompositeCandidate creates a new composite candidate
func NewCompositeCandidate(id string, markers []domain.Marker, spanStart, spanEnd int, sentenceIDs []int) *CompositeCandidate {
	spanLength := spanEnd - spanStart
	density := 0.0
	if spanLength > 0 {
		density = float64(len(markers)) / float64(spanLength)
	}

	// Calculate type diversity
	typeCount := make(map[string]int)
	for _, marker := range markers {
		typeCount[marker.Type]++
	}
	typeDiversity := float64(len(typeCount)) / float64(len(markers))

	// Find primary type
	primaryType := ""
	maxCount := 0
	for t, count := range typeCount {
		if count > maxCount {
			maxCount = count
			primaryType = t
		}
	}

	return &CompositeCandidate{
		ID:            id,
		Markers:       markers,
		SpanStart:     spanStart,
		SpanEnd:       spanEnd,
		SpanLength:    spanLength,
		SentenceIDs:   sentenceIDs,
		Density:       density,
		TypeDiversity: typeDiversity,
		MarkerCount:   len(markers),
		PrimaryType:   primaryType,
		Confidence:    0,
		Score:         0,
		Metadata:      make(map[string]interface{}),
		QualityBucket: "medium",
		HumanInLoop:   false,
	}
}

// AddMarker adds a marker to the candidate
func (cc *CompositeCandidate) AddMarker(marker domain.Marker) {
	cc.Markers = append(cc.Markers, marker)
	cc.MarkerCount++

	// Update span if needed
	if marker.Start < cc.SpanStart {
		cc.SpanStart = marker.Start
	}
	if marker.End > cc.SpanEnd {
		cc.SpanEnd = marker.End
	}

	cc.SpanLength = cc.SpanEnd - cc.SpanStart
	if cc.SpanLength > 0 {
		cc.Density = float64(cc.MarkerCount) / float64(cc.SpanLength)
	}

	// Update type diversity
	typeCount := make(map[string]int)
	for _, m := range cc.Markers {
		typeCount[m.Type]++
	}
	cc.TypeDiversity = float64(len(typeCount)) / float64(cc.MarkerCount)

	// Update primary type
	maxCount := 0
	for t, count := range typeCount {
		if count > maxCount {
			maxCount = count
			cc.PrimaryType = t
		}
	}
}

// AddMetadata adds metadata
func (cc *CompositeCandidate) AddMetadata(key string, value interface{}) {
	if cc.Metadata == nil {
		cc.Metadata = make(map[string]interface{})
	}
	cc.Metadata[key] = value
}

// Overlaps checks if this candidate overlaps with another candidate
func (cc *CompositeCandidate) Overlaps(other *CompositeCandidate, threshold float64) bool {
	overlapStart := domain.Max(cc.SpanStart, other.SpanStart)
	overlapEnd := domain.Min(cc.SpanEnd, other.SpanEnd)

	if overlapStart >= overlapEnd {
		return false
	}

	overlapLength := overlapEnd - overlapStart
	minSpan := domain.Min(cc.SpanLength, other.SpanLength)

	return float64(overlapLength)/float64(minSpan) >= threshold
}

// ContainsMarker checks if the candidate contains a specific marker
func (cc *CompositeCandidate) ContainsMarker(markerID string) bool {
	for _, m := range cc.Markers {
		if m.ID == markerID {
			return true
		}
	}
	return false
}

// GetMarkerTypes returns all unique marker types in the candidate
func (cc *CompositeCandidate) GetMarkerTypes() []string {
	typeSet := make(map[string]bool)
	for _, m := range cc.Markers {
		typeSet[m.Type] = true
	}

	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	return types
}

// GetAverageConfidence calculates the average confidence of markers (float for display)
func (cc *CompositeCandidate) GetAverageConfidence() float64 {
	if cc.MarkerCount == 0 {
		return 0.0
	}

	var total int64
	for _, m := range cc.Markers {
		total += m.Confidence
	}
	return float64(total) / float64(cc.MarkerCount*FixedPointScale)
}