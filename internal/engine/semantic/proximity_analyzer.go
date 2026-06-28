package semantic

import (
    "fmt"
    "math"
    "sort"
    "strings"
    "sync"
    "time"

    "semantic-analyzer/internal/domain"
    "semantic-analyzer/pkg/config"
)


// ProximityAnalyzer analyzes semantic proximity between markers and entities
type ProximityAnalyzer struct {
    config         config.SemanticRulesConfig
    networks       map[string]map[string]bool // network -> word -> exists
    wordToNetworks map[string][]string        // word -> list of networks
    auditEvents    []domain.AuditEvent
    mu             sync.RWMutex
    traceID        string // provenance
}

// NewProximityAnalyzer creates a new proximity analyzer with trace ID
func NewProximityAnalyzer(cfg config.SemanticRulesConfig, traceID string) *ProximityAnalyzer {
    analyzer := &ProximityAnalyzer{
        config:         cfg,
        networks:       make(map[string]map[string]bool),
        wordToNetworks: make(map[string][]string),
        auditEvents:    []domain.AuditEvent{},
        traceID:        traceID,
    }

    // Build network index for fast lookup
    analyzer.buildNetworkIndex()

    return analyzer
}

// buildNetworkIndex builds indexes for fast network lookups
func (pa *ProximityAnalyzer) buildNetworkIndex() {
    for networkName, words := range pa.config.Networks {
        // Initialize network map
        pa.networks[networkName] = make(map[string]bool)

        // Add words to network
        for _, word := range words {
            lowerWord := strings.ToLower(word)
            pa.networks[networkName][lowerWord] = true

            // Add to reverse index
            if _, exists := pa.wordToNetworks[lowerWord]; !exists {
                pa.wordToNetworks[lowerWord] = []string{}
            }

            // Check if network already in list
            found := false
            for _, n := range pa.wordToNetworks[lowerWord] {
                if n == networkName {
                    found = true
                    break
                }
            }

            if !found {
                pa.wordToNetworks[lowerWord] = append(pa.wordToNetworks[lowerWord], networkName)
            }
        }
    }
}

// CalculateSemanticRelatedness calculates semantic relatedness between two markers
// Returns fixed-point integer (scaled by FixedPointScale)
func (pa *ProximityAnalyzer) CalculateSemanticRelatedness(marker1, marker2 domain.Marker) int64 {
    startTime := time.Now()

    // Create audit event with trace ID
    auditEvent := domain.NewAuditEvent("proximity_analyzer", "calculate_relatedness")
    auditEvent.AddData("marker1_id", marker1.ID)
    auditEvent.AddData("marker2_id", marker2.ID)
    auditEvent.AddData("marker1_type", marker1.Type)
    auditEvent.AddData("marker2_type", marker2.Type)
    auditEvent.AddData("trace_id", pa.traceID)
    pa.addAuditEvent(auditEvent)

    // Calculate different aspects of relatedness (as float64)
    networkScore := pa.calculateNetworkRelatedness(marker1, marker2)
    typeScore := pa.calculateTypeSimilarity(marker1, marker2)
    contextScore := pa.calculateContextOverlap(marker1, marker2)

    // Weighted combination
    totalScore := (networkScore * pa.config.ProximityWeights.NetworkMatch) +
        (typeScore * pa.config.ProximityWeights.TypeSimilarity) +
        (contextScore * pa.config.ProximityWeights.ContextOverlap)

    // Normalize to [0, 1]
    normalizedScore := math.Min(math.Max(totalScore, 0.0), 1.0)

    // Convert to fixed-point
    fixedScore := int64(normalizedScore * float64(FixedPointScale))

    // Create completion audit event with trace ID
    endEvent := domain.NewAuditEvent("proximity_analyzer", "relatedness_calculated")
    endEvent.AddData("marker1_id", marker1.ID)
    endEvent.AddData("marker2_id", marker2.ID)
    endEvent.AddData("network_score", networkScore)
    endEvent.AddData("type_score", typeScore)
    endEvent.AddData("context_score", contextScore)
    endEvent.AddData("final_score_float", normalizedScore)
    endEvent.AddData("final_score_fixed", fixedScore)
    endEvent.AddData("weights_applied", pa.config.ProximityWeights)
    endEvent.AddData("trace_id", pa.traceID)
    endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
    endEvent.SetDuration(startTime)
    pa.addAuditEvent(endEvent)

    return fixedScore
}

// calculateNetworkRelatedness calculates relatedness based on semantic networks
func (pa *ProximityAnalyzer) calculateNetworkRelatedness(marker1, marker2 domain.Marker) float64 {
    // Extract words from marker texts
    words1 := pa.extractWords(marker1.TextSpan)
    words2 := pa.extractWords(marker2.TextSpan)

    if len(words1) == 0 || len(words2) == 0 {
        return 0.0
    }

    // Find common networks
    networks1 := pa.getNetworksForWords(words1)
    networks2 := pa.getNetworksForWords(words2)

    // Calculate Jaccard similarity between network sets
    intersection := 0
    for network := range networks1 {
        if networks2[network] {
            intersection++
        }
    }

    union := len(networks1) + len(networks2) - intersection

    if union == 0 {
        return 0.0
    }

    return float64(intersection) / float64(union)
}

// calculateTypeSimilarity calculates similarity based on marker types
func (pa *ProximityAnalyzer) calculateTypeSimilarity(marker1, marker2 domain.Marker) float64 {
    // Simple type matching
    if marker1.Type == marker2.Type {
        return 1.0
    }

    // Check for semantic type relationships
    typeRelations := map[string][]string{
        "CONTACT":    {"EMAIL", "PHONE", "ADDRESS"},
        "PRIORITY":   {"IMPORTANT", "URGENT", "CRITICAL"},
        "ACTION":     {"TASK", "TODO", "FOLLOWUP"},
        "PERSON":     {"CONTACT", "NAME"},
        "ORGANIZATION": {"COMPANY", "BUSINESS"},
    }

    // Check if types are related
    for mainType, relatedTypes := range typeRelations {
        isMarker1Main := marker1.Type == mainType
        isMarker2Main := marker2.Type == mainType

        if isMarker1Main {
            for _, related := range relatedTypes {
                if marker2.Type == related {
                    return 0.7
                }
            }
        }

        if isMarker2Main {
            for _, related := range relatedTypes {
                if marker1.Type == related {
                    return 0.7
                }
            }
        }
    }

    return 0.0
}

// calculateContextOverlap calculates overlap in context
func (pa *ProximityAnalyzer) calculateContextOverlap(marker1, marker2 domain.Marker) float64 {
    // If markers have context, check for word overlap
    if marker1.Context == "" || marker2.Context == "" {
        return 0.0
    }

    words1 := pa.extractWords(marker1.Context)
    words2 := pa.extractWords(marker2.Context)

    if len(words1) == 0 || len(words2) == 0 {
        return 0.0
    }

    // Calculate word overlap
    wordSet1 := make(map[string]bool)
    for _, word := range words1 {
        wordSet1[word] = true
    }

    commonWords := 0
    for _, word := range words2 {
        if wordSet1[word] {
            commonWords++
        }
    }

    // Calculate overlap ratio
    minLength := min(len(words1), len(words2))
    if minLength == 0 {
        return 0.0
    }

    return float64(commonWords) / float64(minLength)
}

// extractWords extracts words from text
func (pa *ProximityAnalyzer) extractWords(text string) []string {
    // Simple word extraction - in production, use proper tokenization
    words := strings.Fields(strings.ToLower(text))

    // Remove common stop words and punctuation
    var filtered []string
    stopWords := map[string]bool{
        "the": true, "a": true, "an": true, "and": true, "or": true,
        "but": true, "in": true, "on": true, "at": true, "to": true,
        "for": true, "of": true, "with": true, "by": true, "is": true,
        "are": true, "was": true, "were": true, "be": true, "been": true,
    }

    for _, word := range words {
        // Remove punctuation
        cleaned := strings.Trim(word, ".,;:!?\"'()[]{}")

        // Skip stop words and very short words
        if len(cleaned) > 2 && !stopWords[cleaned] {
            filtered = append(filtered, cleaned)
        }
    }

    return filtered
}

// getNetworksForWords finds all networks that contain the given words
func (pa *ProximityAnalyzer) getNetworksForWords(words []string) map[string]bool {
    networks := make(map[string]bool)

    for _, word := range words {
        if wordNetworks, exists := pa.wordToNetworks[word]; exists {
            for _, network := range wordNetworks {
                networks[network] = true
            }
        }
    }

    return networks
}

// FindRelatedMarkers finds markers related to a given marker
// threshold is a float64 in [0,1] and will be converted to fixed-point internally
func (pa *ProximityAnalyzer) FindRelatedMarkers(target domain.Marker, allMarkers []domain.Marker, threshold float64) []RelatedMarker {
    startTime := time.Now()

    // Convert threshold to fixed-point
    thresholdFixed := int64(threshold * float64(FixedPointScale))

    // Create audit event with trace ID
    auditEvent := domain.NewAuditEvent("proximity_analyzer", "find_related_markers")
    auditEvent.AddData("target_marker_id", target.ID)
    auditEvent.AddData("total_markers", len(allMarkers))
    auditEvent.AddData("threshold", threshold)
    auditEvent.AddData("threshold_fixed", thresholdFixed)
    auditEvent.AddData("trace_id", pa.traceID)
    pa.addAuditEvent(auditEvent)

    var related []RelatedMarker

    for _, marker := range allMarkers {
        if marker.ID == target.ID {
            continue // Skip self
        }

        relatedness := pa.CalculateSemanticRelatedness(target, marker)
        if relatedness >= thresholdFixed {
            related = append(related, RelatedMarker{
                Marker:      marker,
                Relatedness: float64(relatedness) / float64(FixedPointScale), // convert back to float for output
                Reason:      pa.explainRelatedness(target, marker, float64(relatedness)/float64(FixedPointScale)),
            })
        }
    }

    // Sort by relatedness (descending)
    sort.Slice(related, func(i, j int) bool {
        return related[i].Relatedness > related[j].Relatedness
    })

    // Create completion audit event with trace ID
    endEvent := domain.NewAuditEvent("proximity_analyzer", "related_markers_found")
    endEvent.AddData("target_marker_id", target.ID)
    endEvent.AddData("related_count", len(related))
    if len(related) > 0 {
        endEvent.AddData("top_relatedness", related[0].Relatedness)
    }
    endEvent.AddData("trace_id", pa.traceID)
    endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
    endEvent.SetDuration(startTime)
    pa.addAuditEvent(endEvent)

    return related
}

// explainRelatedness generates an explanation for why markers are related
func (pa *ProximityAnalyzer) explainRelatedness(marker1, marker2 domain.Marker, score float64) string {
    var reasons []string

    // Network overlap explanation
    words1 := pa.extractWords(marker1.TextSpan)
    words2 := pa.extractWords(marker2.TextSpan)

    networks1 := pa.getNetworksForWords(words1)
    networks2 := pa.getNetworksForWords(words2)

    commonNetworks := []string{}
    for network := range networks1 {
        if networks2[network] {
            commonNetworks = append(commonNetworks, network)
        }
    }

    if len(commonNetworks) > 0 {
        reasons = append(reasons, fmt.Sprintf("Share semantic networks: %s", strings.Join(commonNetworks, ", ")))
    }

    // Type similarity explanation
    if marker1.Type == marker2.Type {
        reasons = append(reasons, "Same marker type")
    } else if pa.calculateTypeSimilarity(marker1, marker2) > 0.5 {
        reasons = append(reasons, "Related marker types")
    }

    // Context overlap explanation
    contextScore := pa.calculateContextOverlap(marker1, marker2)
    if contextScore > 0.3 {
        reasons = append(reasons, fmt.Sprintf("Context overlap: %.0f%%", contextScore*100))
    }

    if len(reasons) == 0 {
        return "General semantic similarity"
    }

    return strings.Join(reasons, "; ")
}

// RelatedMarker represents a marker with its relatedness score
type RelatedMarker struct {
    Marker      domain.Marker `json:"marker"`
    Relatedness float64       `json:"relatedness"`
    Reason      string        `json:"reason"`
}

// CalculateClusterCohesion calculates cohesion within a cluster of markers
// Returns a float64 in [0,1] representing average relatedness
func (pa *ProximityAnalyzer) CalculateClusterCohesion(markers []domain.Marker) float64 {
    if len(markers) <= 1 {
        return 1.0 // Single marker cluster is perfectly cohesive
    }

    var totalRelatedness int64
    pairCount := 0

    // Calculate average relatedness between all pairs (using fixed-point)
    for i := 0; i < len(markers); i++ {
        for j := i + 1; j < len(markers); j++ {
            relatedness := pa.CalculateSemanticRelatedness(markers[i], markers[j])
            totalRelatedness += relatedness
            pairCount++
        }
    }

    if pairCount == 0 {
        return 0.0
    }

    // Convert average back to float
    avgFixed := totalRelatedness / int64(pairCount)
    return float64(avgFixed) / float64(FixedPointScale)
}

// FindSemanticClusters finds clusters of semantically related markers
// threshold is a float64 in [0,1] and will be converted to fixed-point internally
func (pa *ProximityAnalyzer) FindSemanticClusters(markers []domain.Marker, threshold float64) [][]domain.Marker {
    thresholdFixed := int64(threshold * float64(FixedPointScale))

    // Simple clustering algorithm
    var clusters [][]domain.Marker
    assigned := make(map[string]bool)

    for i, marker := range markers {
        if assigned[marker.ID] {
            continue
        }

        // Start a new cluster with this marker
        cluster := []domain.Marker{marker}
        assigned[marker.ID] = true

        // Find related markers
        for j := i + 1; j < len(markers); j++ {
            if assigned[markers[j].ID] {
                continue
            }

            relatedness := pa.CalculateSemanticRelatedness(marker, markers[j])
            if relatedness >= thresholdFixed {
                cluster = append(cluster, markers[j])
                assigned[markers[j].ID] = true
            }
        }

        clusters = append(clusters, cluster)
    }

    return clusters
}

// addAuditEvent adds an audit event with trace ID
func (pa *ProximityAnalyzer) addAuditEvent(event *domain.AuditEvent) {
    // Ensure trace_id is set (should already be, but add as fallback)
    if _, ok := event.DataSnapshot["trace_id"]; !ok {
        event.AddData("trace_id", pa.traceID)
    }
    pa.mu.Lock()
    defer pa.mu.Unlock()
    pa.auditEvents = append(pa.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (pa *ProximityAnalyzer) GetAuditEvents() []domain.AuditEvent {
    pa.mu.RLock()
    defer pa.mu.RUnlock()
    return pa.auditEvents
}

