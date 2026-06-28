package composite

import (
	"sort"
	"strings"
	"time"

	"semantic-analyzer/internal/domain"
	"semantic-analyzer/pkg/determinism"
)

// Selector selects top composite candidates
type Selector struct {
	seedManager *determinism.SeedManager
	auditEvents []domain.AuditEvent
}

// NewSelector creates a new composite selector
func NewSelector(seedManager *determinism.SeedManager) *Selector {
	return &Selector{
		seedManager: seedManager,
		auditEvents: []domain.AuditEvent{},
	}
}

// SelectTopCandidates selects the top candidates based on score
func (s *Selector) SelectTopCandidates(candidates []CompositeCandidate, maxCount int) []CompositeCandidate {
	startTime := time.Now()

	auditEvent := domain.NewAuditEvent("composite_selector", "select_top_candidates")
	auditEvent.AddData("input_candidates", len(candidates))
	auditEvent.AddData("max_count", maxCount)
	s.addAuditEvent(auditEvent)

	if len(candidates) == 0 {
		return candidates
	}

	// Удаление дубликатов по составу маркеров (ключевое исправление)
	uniqueCandidates := s.removeDuplicateCandidates(candidates)

	sortedCandidates := make([]CompositeCandidate, len(uniqueCandidates))
	copy(sortedCandidates, uniqueCandidates)

	s.deterministicSort(sortedCandidates)

	selectedCount := domain.Min(maxCount, len(sortedCandidates))
	if selectedCount == 0 {
		selectedCount = len(sortedCandidates)
	}
	selected := sortedCandidates[:selectedCount]

	for i := range selected {
		selected[i].Metadata["selection_rank"] = i + 1
		selected[i].Metadata["selection_timestamp"] = time.Now().Format(time.RFC3339)
		selected[i].Metadata["total_candidates"] = len(candidates)
	}

	endEvent := domain.NewAuditEvent("composite_selector", "candidates_selected")
	endEvent.AddData("selected_count", len(selected))
	topScore := int64(0)
	if len(selected) > 0 {
		topScore = selected[0].Score
	}
	endEvent.AddData("top_score", domain.FromFixedPoint(topScore))
	endEvent.AddData("average_selected_score", domain.FromFixedPoint(s.calculateAverageScore(selected)))
	endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
	endEvent.SetDuration(startTime)
	s.addAuditEvent(endEvent)

	return selected
}

// removeDuplicateCandidates удаляет кандидатов с одинаковым набором ID маркеров (оставляет с наивысшим Score)
func (s *Selector) removeDuplicateCandidates(candidates []CompositeCandidate) []CompositeCandidate {
	type group struct {
		indices  []int
		maxScore int64
	}
	groups := make(map[string]*group)
	for i, cand := range candidates {
		ids := make([]string, len(cand.Markers))
		for j, m := range cand.Markers {
			ids[j] = m.ID
		}
		sort.Strings(ids)
		key := strings.Join(ids, "|")
		if grp, exists := groups[key]; exists {
			grp.indices = append(grp.indices, i)
			if cand.Score > grp.maxScore {
				grp.maxScore = cand.Score
			}
		} else {
			groups[key] = &group{
				indices:  []int{i},
				maxScore: cand.Score,
			}
		}
	}
	var unique []CompositeCandidate
	for _, grp := range groups {
		bestIdx := grp.indices[0]
		for _, idx := range grp.indices {
			if candidates[idx].Score == grp.maxScore {
				bestIdx = idx
				break
			}
		}
		unique = append(unique, candidates[bestIdx])
	}
	return unique
}

// deterministicSort sorts candidates deterministically
func (s *Selector) deterministicSort(candidates []CompositeCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	s.breakTiesDeterministically(candidates)
}

// breakTiesDeterministically breaks ties using deterministic rules
func (s *Selector) breakTiesDeterministically(candidates []CompositeCandidate) {
	scoreGroups := make(map[int64][]*CompositeCandidate)
	for i := range candidates {
		score := candidates[i].Score
		scoreGroups[score] = append(scoreGroups[score], &candidates[i])
	}

	for _, group := range scoreGroups {
		if len(group) > 1 {
			sort.SliceStable(group, func(i, j int) bool {
				a := group[i]
				b := group[j]

				if a.MarkerCount != b.MarkerCount {
					return a.MarkerCount > b.MarkerCount
				}

				if a.Density != b.Density {
					return a.Density > b.Density
				}

				if a.TypeDiversity != b.TypeDiversity {
					return a.TypeDiversity > b.TypeDiversity
				}

				if a.SpanLength != b.SpanLength {
					return a.SpanLength < b.SpanLength
				}

				hashA := s.seedManager.DeterministicHash(a.ID)
				hashB := s.seedManager.DeterministicHash(b.ID)
				return hashA < hashB
			})
		}
	}
}

// FilterByScoreThreshold filters candidates by minimum score
func (s *Selector) FilterByScoreThreshold(candidates []CompositeCandidate, minScore float64) []CompositeCandidate {
	var filtered []CompositeCandidate
	minScoreFixed := domain.ToFixedPoint(minScore)

	for _, candidate := range candidates {
		if candidate.Score >= minScoreFixed {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

// RemoveRedundantCandidates removes redundant candidates
func (s *Selector) RemoveRedundantCandidates(candidates []CompositeCandidate, redundancyThreshold float64) []CompositeCandidate {
	if len(candidates) <= 1 {
		return candidates
	}

	sorted := make([]CompositeCandidate, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	var unique []CompositeCandidate
	uniqueSet := make(map[string]bool)

	for _, candidate := range sorted {
		isRedundant := false

		for _, u := range unique {
			if s.isRedundant(&candidate, &u, redundancyThreshold) {
				isRedundant = true
				break
			}
		}

		if !isRedundant && !uniqueSet[candidate.ID] {
			unique = append(unique, candidate)
			uniqueSet[candidate.ID] = true
		}
	}

	return unique
}

// isRedundant checks if one candidate is redundant compared to another
func (s *Selector) isRedundant(candidate, reference *CompositeCandidate, threshold float64) bool {
	markerSet1 := make(map[string]bool)
	for _, marker := range reference.Markers {
		markerSet1[marker.ID] = true
	}

	commonMarkers := 0
	for _, marker := range candidate.Markers {
		if markerSet1[marker.ID] {
			commonMarkers++
		}
	}

	overlapRatio := float64(commonMarkers) / float64(domain.Min(len(candidate.Markers), len(reference.Markers)))

	spanOverlapStart := domain.Max(candidate.SpanStart, reference.SpanStart)
	spanOverlapEnd := domain.Min(candidate.SpanEnd, reference.SpanEnd)
	spanOverlap := 0.0

	if spanOverlapEnd > spanOverlapStart {
		span1 := candidate.SpanEnd - candidate.SpanStart
		span2 := reference.SpanEnd - reference.SpanStart
		minSpan := domain.Min(span1, span2)
		if minSpan > 0 {
			spanOverlap = float64(spanOverlapEnd-spanOverlapStart) / float64(minSpan)
		}
	}

	return overlapRatio >= threshold && spanOverlap >= 0.5
}

// SelectDiverseCandidates selects a diverse set of candidates
func (s *Selector) SelectDiverseCandidates(candidates []CompositeCandidate, maxCount int, diversityWeight float64) []CompositeCandidate {
	if len(candidates) == 0 {
		return candidates
	}

	diversityScores := s.calculateDiversityScores(candidates)

	combinedScores := make([]int64, len(candidates))
	for i := range candidates {
		original := candidates[i].Score
		diversity := domain.ToFixedPoint(diversityScores[i])
		combined := original*(domain.FixedPointScale-int64(diversityWeight*float64(domain.FixedPointScale))) / domain.FixedPointScale +
			diversity*int64(diversityWeight*float64(domain.FixedPointScale)) / domain.FixedPointScale
		combinedScores[i] = combined
	}

	sortedIndices := make([]int, len(candidates))
	for i := range sortedIndices {
		sortedIndices[i] = i
	}

	sort.Slice(sortedIndices, func(i, j int) bool {
		return combinedScores[sortedIndices[i]] > combinedScores[sortedIndices[j]]
	})

	selectedCount := domain.Min(maxCount, len(candidates))
	selected := make([]CompositeCandidate, selectedCount)

	for i := 0; i < selectedCount; i++ {
		idx := sortedIndices[i]
		selected[i] = candidates[idx]
		selected[i].Metadata["diversity_score"] = diversityScores[idx]
		selected[i].Metadata["combined_score"] = domain.FromFixedPoint(combinedScores[idx])
	}

	return selected
}

// calculateDiversityScores calculates diversity scores for candidates
func (s *Selector) calculateDiversityScores(candidates []CompositeCandidate) []float64 {
	scores := make([]float64, len(candidates))

	for i, candidate := range candidates {
		diversityScore := candidate.TypeDiversity

		similarityPenalty := 0.0
		for j, other := range candidates {
			if i == j {
				continue
			}
			similarity := s.calculateSimilarity(&candidate, &other)
			similarityPenalty += similarity
		}

		if len(candidates) > 1 {
			similarityPenalty /= float64(len(candidates) - 1)
		}

		scores[i] = diversityScore * (1.0 - similarityPenalty)
	}

	return scores
}

// calculateSimilarity calculates similarity between two candidates
func (s *Selector) calculateSimilarity(candidate1, candidate2 *CompositeCandidate) float64 {
	// Calculate marker overlap
	markerSet1 := make(map[string]bool)
	for _, marker := range candidate1.Markers {
		markerSet1[marker.ID] = true
	}

	commonMarkers := 0
	for _, marker := range candidate2.Markers {
		if markerSet1[marker.ID] {
			commonMarkers++
		}
	}

	markerOverlap := float64(commonMarkers) / float64(domain.Min(len(candidate1.Markers), len(candidate2.Markers)))

	// Calculate type overlap
	typeSet1 := make(map[string]bool)
	for _, marker := range candidate1.Markers {
		typeSet1[marker.Type] = true
	}

	commonTypes := 0
	for _, marker := range candidate2.Markers {
		if typeSet1[marker.Type] {
			commonTypes++
		}
	}

	typeOverlap := float64(commonTypes) / float64(domain.Min(len(typeSet1), len(candidate2.GetMarkerTypes())))

	// Calculate span overlap
	spanOverlapStart := domain.Max(candidate1.SpanStart, candidate2.SpanStart)
	spanOverlapEnd := domain.Min(candidate1.SpanEnd, candidate2.SpanEnd)
	spanOverlap := 0.0

	if spanOverlapEnd > spanOverlapStart {
		span1 := candidate1.SpanEnd - candidate1.SpanStart
		span2 := candidate2.SpanEnd - candidate2.SpanStart
		minSpan := domain.Min(span1, span2)
		if minSpan > 0 {
			spanOverlap = float64(spanOverlapEnd-spanOverlapStart) / float64(minSpan)
		}
	}

	similarity := (markerOverlap * 0.4) + (typeOverlap * 0.3) + (spanOverlap * 0.3)
	return similarity
}

// calculateAverageScore calculates the average score of candidates (fixed-point)
func (s *Selector) calculateAverageScore(candidates []CompositeCandidate) int64 {
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
func (s *Selector) addAuditEvent(event *domain.AuditEvent) {
	s.auditEvents = append(s.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (s *Selector) GetAuditEvents() []domain.AuditEvent {
	return s.auditEvents
}