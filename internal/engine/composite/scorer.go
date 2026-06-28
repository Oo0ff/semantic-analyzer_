package composite

import (
	"math"
	"sort"
	"time"

	"semantic-analyzer/internal/domain"
	"semantic-analyzer/pkg/config"
)

// Scorer calculates scores for composite candidates
type Scorer struct {
	config      config.CompositeRulesConfig
	auditEvents []domain.AuditEvent
}

// NewScorer creates a new composite scorer
func NewScorer(cfg config.CompositeRulesConfig) *Scorer {
	return &Scorer{
		config:      cfg,
		auditEvents: []domain.AuditEvent{},
	}
}

// ScoreCandidate calculates a score for a composite candidate (fixed‑point 0..FixedPointScale)
func (s *Scorer) ScoreCandidate(candidate *CompositeCandidate) int64 {
	startTime := time.Now()

	auditEvent := domain.NewAuditEvent("composite_scorer", "score_candidate")
	auditEvent.AddData("candidate_id", candidate.ID)
	auditEvent.AddData("marker_count", candidate.MarkerCount)
	auditEvent.AddData("span_length", candidate.SpanLength)
	s.addAuditEvent(auditEvent)

	// Все компоненты возвращают fixed‑point (0..FixedPointScale)
	markerCountScore := s.scoreMarkerCount(candidate)
	densityScore := s.scoreDensity(candidate)
	spanCompactnessScore := s.scoreSpanCompactness(candidate)
	typeDiversityScore := s.scoreTypeDiversity(candidate)
	typeWeightScore := s.scoreTypeWeights(candidate)
	confidenceScore := s.scoreConfidence(candidate)

	// Веса из конфига – дробные числа, их сумма не обязательно равна 1.
	// Нормируем четыре основных веса, чтобы сумма была 1.
	w := s.config.SelectionWeights
	sumWeights := w.MarkerCount + w.Density + w.SpanCompactness + w.TypeDiversity
	if sumWeights <= 0 {
		sumWeights = 1.0 // защита от нуля
	}

	// Вычисляем взвешенную основных компонент (в fixed‑point)
	// Используем float64 для точности, потом переводим в int64
	weightedMain := float64(markerCountScore)*(w.MarkerCount/sumWeights) +
		float64(densityScore)*(w.Density/sumWeights) +
		float64(spanCompactnessScore)*(w.SpanCompactness/sumWeights) +
		float64(typeDiversityScore)*(w.TypeDiversity/sumWeights)

	// Дополнительные компоненты с фиксированным весом
	// 80% – основные, 10% – тип-вес, 10% – уверенность
	totalFloat := weightedMain*0.8 + float64(typeWeightScore)*0.1 + float64(confidenceScore)*0.1

	// Преобразуем в целое, ограничиваем диапазон
	score := int64(math.Round(totalFloat))
	if score < 0 {
		score = 0
	}
	if score > domain.FixedPointScale {
		score = domain.FixedPointScale
	}

	// Обновляем кандидата
	candidate.Score = score
	candidate.Confidence = score

	// Метрики для аудита (в читаемом виде)
	candidate.Metadata["scoring_components"] = map[string]interface{}{
		"marker_count":      domain.FromFixedPoint(markerCountScore),
		"density":           domain.FromFixedPoint(densityScore),
		"span_compactness":  domain.FromFixedPoint(spanCompactnessScore),
		"type_diversity":    domain.FromFixedPoint(typeDiversityScore),
		"type_weight":       domain.FromFixedPoint(typeWeightScore),
		"confidence":        domain.FromFixedPoint(confidenceScore),
		"weights_applied":   s.config.SelectionWeights,
	}

	endEvent := domain.NewAuditEvent("composite_scorer", "candidate_scored")
	endEvent.AddData("candidate_id", candidate.ID)
	endEvent.AddData("final_score", domain.FromFixedPoint(score))
	endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
	endEvent.SetDuration(startTime)
	s.addAuditEvent(endEvent)

	return score
}

// scoreMarkerCount scores based on the number of markers (fixed-point)
func (s *Scorer) scoreMarkerCount(candidate *CompositeCandidate) int64 {
	idealMin := 3
	idealMax := 6

	if candidate.MarkerCount < 2 {
		return domain.ToFixedPoint(0.1)
	}
	if candidate.MarkerCount >= idealMin && candidate.MarkerCount <= idealMax {
		return domain.FixedPointScale
	}

	if candidate.MarkerCount > idealMax {
		excess := candidate.MarkerCount - idealMax
		penalty := domain.ToFixedPoint(float64(excess) * 0.1)
		score := domain.FixedPointScale - penalty
		if score < domain.ToFixedPoint(0.3) {
			return domain.ToFixedPoint(0.3)
		}
		return score
	}

	return domain.ToFixedPoint(0.5)
}

// scoreDensity scores based on marker density (fixed-point)
func (s *Scorer) scoreDensity(candidate *CompositeCandidate) int64 {
	if candidate.SpanLength == 0 {
		return 0
	}

	baselineDensity := 0.01
	normalizedDensity := candidate.Density / baselineDensity

	x := normalizedDensity - 1.0
	score := 1.0 / (1.0 + math.Exp(-5.0*x))

	return domain.ToFixedPoint(math.Min(math.Max(score, 0.0), 1.0))
}

// scoreSpanCompactness scores based on how compact the span is (fixed-point)
func (s *Scorer) scoreSpanCompactness(candidate *CompositeCandidate) int64 {
	if candidate.SpanLength == 0 || candidate.MarkerCount < 2 {
		return 0
	}

	var totalDistance float64
	markerPositions := make([]int, candidate.MarkerCount)

	for i, marker := range candidate.Markers {
		markerPositions[i] = (marker.Start + marker.End) / 2
	}

	sort.Ints(markerPositions)

	for i := 1; i < len(markerPositions); i++ {
		totalDistance += float64(markerPositions[i] - markerPositions[i-1])
	}

	avgDistance := totalDistance / float64(len(markerPositions)-1)

	if avgDistance == 0 {
		return domain.FixedPointScale
	}

	compactness := float64(candidate.SpanLength) / (avgDistance * float64(candidate.MarkerCount))

	x := compactness - 0.5
	score := 1.0 / (1.0 + math.Exp(-10.0*x))

	return domain.ToFixedPoint(math.Min(math.Max(score, 0.0), 1.0))
}

// scoreTypeDiversity scores based on diversity of marker types (fixed-point)
func (s *Scorer) scoreTypeDiversity(candidate *CompositeCandidate) int64 {
	return domain.ToFixedPoint(candidate.TypeDiversity)
}

// scoreTypeWeights scores based on the importance of marker types (fixed-point)
func (s *Scorer) scoreTypeWeights(candidate *CompositeCandidate) int64 {
	if candidate.MarkerCount == 0 {
		return 0
	}

	var totalWeight float64
	for _, marker := range candidate.Markers {
		if w, ok := s.config.TypeWeights[marker.Type]; ok {
			totalWeight += w
		} else {
			totalWeight += 0.3
		}
	}

	avgWeight := totalWeight / float64(candidate.MarkerCount)
	return domain.ToFixedPoint(math.Min(math.Max(avgWeight, 0.0), 1.0))
}

// scoreConfidence scores based on the confidence of individual markers (fixed-point)
func (s *Scorer) scoreConfidence(candidate *CompositeCandidate) int64 {
	return candidate.Confidence // уже fixed‑point
}

// ScoreAllCandidates scores all candidates
func (s *Scorer) ScoreAllCandidates(candidates []CompositeCandidate) []CompositeCandidate {
	startTime := time.Now()

	auditEvent := domain.NewAuditEvent("composite_scorer", "score_all_candidates")
	auditEvent.AddData("candidate_count", len(candidates))
	s.addAuditEvent(auditEvent)

	scoredCandidates := make([]CompositeCandidate, len(candidates))

	for i := range candidates {
		cand := &candidates[i]
		score := s.ScoreCandidate(cand)
		cand.Score = score
		cand.Confidence = score
		scoredCandidates[i] = *cand
	}

	sort.Slice(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].Score == scoredCandidates[j].Score {
			return scoredCandidates[i].MarkerCount > scoredCandidates[j].MarkerCount
		}
		return scoredCandidates[i].Score > scoredCandidates[j].Score
	})

	endEvent := domain.NewAuditEvent("composite_scorer", "all_candidates_scored")
	endEvent.AddData("candidate_count", len(scoredCandidates))
	endEvent.AddData("average_score", domain.FromFixedPoint(s.calculateAverageScore(scoredCandidates)))
	endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
	endEvent.SetDuration(startTime)
	s.addAuditEvent(endEvent)

	return scoredCandidates
}

// calculateAverageScore calculates the average score of candidates (fixed-point)
func (s *Scorer) calculateAverageScore(candidates []CompositeCandidate) int64 {
	if len(candidates) == 0 {
		return 0
	}

	var total int64
	for _, candidate := range candidates {
		total += candidate.Score
	}

	return total / int64(len(candidates))
}

// addAuditEvent adds an audit event
func (s *Scorer) addAuditEvent(event *domain.AuditEvent) {
	s.auditEvents = append(s.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (s *Scorer) GetAuditEvents() []domain.AuditEvent {
	return s.auditEvents
}