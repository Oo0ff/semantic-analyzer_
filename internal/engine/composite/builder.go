package composite

import (
	"fmt"
	"sort"
	"time"

	"semantic-analyzer/internal/domain"
	"semantic-analyzer/pkg/config"
)

// Builder builds composite candidates
type Builder struct {
	config      config.CompositeRulesConfig
	auditEvents []domain.AuditEvent
	traceID     string
}

// NewBuilder creates a new composite builder
func NewBuilder(cfg config.CompositeRulesConfig, traceID string) *Builder {
	return &Builder{
		config:      cfg,
		auditEvents: []domain.AuditEvent{},
		traceID:     traceID,
	}
}

// BuildContextWindows builds composite candidates from atomic markers and sentences
func (b *Builder) BuildContextWindows(markers []domain.Marker, sentences []string) []CompositeCandidate {
	startTime := time.Now()

	auditEvent := domain.NewAuditEvent("composite_builder", "build_context_windows")
	auditEvent.AddData("marker_count", len(markers))
	auditEvent.AddData("sentence_count", len(sentences))
	auditEvent.AddData("trace_id", b.traceID)
	b.addAuditEvent(auditEvent)

	if len(markers) < 2 {
		return []CompositeCandidate{}
	}

	// Sort markers by start position (use stable sort for determinism)
	sortedMarkers := make([]domain.Marker, len(markers))
	copy(sortedMarkers, markers)
	sort.SliceStable(sortedMarkers, func(i, j int) bool {
		return sortedMarkers[i].Start < sortedMarkers[j].Start
	})

	var candidates []CompositeCandidate
	i := 0
	for i < len(sortedMarkers) {
		current := sortedMarkers[i]
		candMarkers := []domain.Marker{current}
		spanStart := current.Start
		spanEnd := current.End
		sentenceIDs := []int{}

		j := i + 1
		for j < len(sortedMarkers) {
			next := sortedMarkers[j]
			if next.Start - spanEnd > b.config.ProximityWindow {
				break
			}
			candMarkers = append(candMarkers, next)
			spanEnd = next.End
			j++
		}

		if len(candMarkers) >= 2 {
			id := fmt.Sprintf("cand_%d", len(candidates))
			cand := *NewCompositeCandidate(id, candMarkers, spanStart, spanEnd, sentenceIDs)

			// Score будет установлен в Scorer — здесь просто инициализируем 0
			cand.Score = 0

			if cand.Density >= b.config.MinDensity && cand.SpanLength <= b.config.MaxSpan {
				candidates = append(candidates, cand)
			}
		}

		i = j
	}

	// Merge similar candidates
	candidates = b.MergeSimilarCandidates(candidates, 0.5)

	endEvent := domain.NewAuditEvent("composite_builder", "context_windows_built")
	endEvent.AddData("candidate_count", len(candidates))
	endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
	endEvent.AddData("trace_id", b.traceID)
	endEvent.SetDuration(startTime)
	b.addAuditEvent(endEvent)

	return candidates
}

// MergeSimilarCandidates merges overlapping candidates
func (b *Builder) MergeSimilarCandidates(candidates []CompositeCandidate, threshold float64) []CompositeCandidate {
	if len(candidates) <= 1 {
		return candidates
	}

	// Sort by start (stable for determinism)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].SpanStart < candidates[j].SpanStart
	})

	var merged []CompositeCandidate
	i := 0
	for i < len(candidates) {
		current := candidates[i]
		j := i + 1
		for j < len(candidates) {
			if !current.Overlaps(&candidates[j], threshold) {
				break
			}
			// Merge markers (avoid duplicates)
			for _, m := range candidates[j].Markers {
				if !current.ContainsMarker(m.ID) {
					current.AddMarker(m)
				}
			}
			current.SpanStart = domain.Min(current.SpanStart, candidates[j].SpanStart)
			current.SpanEnd = domain.Max(current.SpanEnd, candidates[j].SpanEnd)
			j++
		}
		merged = append(merged, current)
		i = j
	}

	return merged
}

// addAuditEvent adds an audit event
func (b *Builder) addAuditEvent(event *domain.AuditEvent) {
	b.auditEvents = append(b.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (b *Builder) GetAuditEvents() []domain.AuditEvent {
	return b.auditEvents
}