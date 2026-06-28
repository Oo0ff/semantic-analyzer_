package semantic

import (
    "crypto/md5"
    "fmt"
    "sync"
    "time"

    "semantic-analyzer/internal/domain"
    "semantic-analyzer/pkg/config"
    composite "semantic-analyzer/internal/engine/composite"
)

// SemanticEngine orchestrates all semantic analysis
type SemanticEngine struct {
    nerEngine          *NEREngine
    proximityAnalyzer  *ProximityAnalyzer
    logicalEvaluator   *LogicalEvaluator
    config             config.SemanticRulesConfig
    auditEvents        []domain.AuditEvent
    mu                 sync.RWMutex
    seed               string
}

// NewSemanticEngine creates a new semantic engine
func NewSemanticEngine(cfg config.SemanticRulesConfig, seed string) (*SemanticEngine, error) {
    nerEngine, err := NewNEREngine(cfg.NER, seed)
    if err != nil {
        return nil, fmt.Errorf("failed to create NER engine: %w", err)
    }

    return &SemanticEngine{
        nerEngine:         nerEngine,
        proximityAnalyzer: NewProximityAnalyzer(cfg, seed),
        logicalEvaluator:  NewLogicalEvaluator(cfg, seed),
        config:            cfg,
        auditEvents:       []domain.AuditEvent{},
        seed:              seed,
    }, nil
}

// Analyze performs comprehensive semantic analysis
func (se *SemanticEngine) Analyze(
    text string,
    atomicMarkers []domain.Marker,
    compositeCandidates []composite.CompositeCandidate,
) (*SemanticAnalysisResult, error) {
    startTime := time.Now()

    auditEvent := domain.NewAuditEvent("semantic_engine", "start_analysis")
    auditEvent.AddData("text_length", len(text))
    auditEvent.AddData("atomic_markers", len(atomicMarkers))
    auditEvent.AddData("composite_candidates", len(compositeCandidates))
    se.addAuditEvent(auditEvent)

    result := &SemanticAnalysisResult{
        Timestamp: time.Now(),
        TextHash:  fmt.Sprintf("%x", md5.Sum([]byte(text))),
    }

    // Step 1: Extract named entities
    entities := se.nerEngine.ExtractEntities(text)
    result.Entities = entities
    se.addAuditEvents(se.nerEngine.GetAuditEvents())

    // Step 2: Analyze semantic proximity
    proximityResults := se.analyzeProximity(atomicMarkers, compositeCandidates, entities)
    result.ProximityAnalysis = proximityResults

    // Step 3: Evaluate logical rules
    context := se.buildContext(text, atomicMarkers, compositeCandidates, entities)
    ruleResults := se.logicalEvaluator.EvaluateAllRules(context)
    result.LogicalRules = ruleResults

    // Step 4: Generate composite markers from logical rules
    compositeMarkers := se.generateCompositeMarkers(ruleResults, context)
    result.GeneratedMarkers = compositeMarkers

    // Step 5: Calculate overall semantic score
    result.SemanticScore = se.calculateSemanticScore(entities, proximityResults, ruleResults)

    endEvent := domain.NewAuditEvent("semantic_engine", "analysis_complete")
    endEvent.AddData("entities_found", len(entities))
    endEvent.AddData("proximity_analyses", len(proximityResults))
    endEvent.AddData("rules_triggered", len(ruleResults))
    endEvent.AddData("composite_markers_generated", len(compositeMarkers))
    endEvent.AddData("semantic_score", result.SemanticScore)
    endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
    endEvent.SetDuration(startTime)
    se.addAuditEvent(endEvent)

    return result, nil
}

// analyzeProximity analyzes semantic proximity between elements
func (se *SemanticEngine) analyzeProximity(
    markers []domain.Marker,
    candidates []composite.CompositeCandidate,
    entities []Entity,
) []ProximityAnalysis {
    var analyses []ProximityAnalysis

    for i, marker := range markers {
        if i >= 10 {
            break
        }
        relatedMarkers := se.proximityAnalyzer.FindRelatedMarkers(marker, markers, 0.3)
        if len(relatedMarkers) > 0 {
            analysis := ProximityAnalysis{
                SourceID:           marker.ID,
                SourceType:         "marker",
                RelatedItems:       relatedMarkers,
                AverageRelatedness: calculateAverageRelatedness(relatedMarkers),
            }
            analyses = append(analyses, analysis)
        }
    }

    for i, entity := range entities {
        if i >= 10 {
            break
        }
        entityMarkers := convertEntitiesToMarkers(entities)
        relatedEntities := se.proximityAnalyzer.FindRelatedMarkers(
            domain.Marker{ID: entity.ID, Type: entity.Type, TextSpan: entity.Text},
            entityMarkers, 0.3)
        if len(relatedEntities) > 0 {
            analysis := ProximityAnalysis{
                SourceID:           entity.ID,
                SourceType:         "entity",
                RelatedItems:       relatedEntities,
                AverageRelatedness: calculateAverageRelatedness(relatedEntities),
            }
            analyses = append(analyses, analysis)
        }
    }

    se.addAuditEvents(se.proximityAnalyzer.GetAuditEvents())
    return analyses
}

// buildContext builds evaluation context from all analysis results
func (se *SemanticEngine) buildContext(
    text string,
    markers []domain.Marker,
    candidates []composite.CompositeCandidate,
    entities []Entity,
) map[string]interface{} {
    context := make(map[string]interface{})
    context["text"] = text
    context["markers"] = markers
    context["entities"] = entities

    var candidateContext []map[string]interface{}
    for _, c := range candidates {
        candidateContext = append(candidateContext, map[string]interface{}{
            "id":           c.ID,
            "primary_type": c.PrimaryType,
            "score":        c.Score,
            "marker_count": c.MarkerCount,
            "span_start":   c.SpanStart,
            "span_end":     c.SpanEnd,
        })
    }
    context["composite_candidates"] = candidateContext
    context["semantic_networks"] = se.config.Networks
    return context
}

// generateCompositeMarkers generates composite markers from triggered logical rules
func (se *SemanticEngine) generateCompositeMarkers(
    ruleResults []RuleResult,
    context map[string]interface{},
) []domain.Marker {
    var markers []domain.Marker
    for _, ruleResult := range ruleResults {
        if ruleResult.Result && ruleResult.OutputType == "COMPOSITE" {
            confidence := domain.ToFixedPoint(ruleResult.Weight) // правильное fixed-point
            marker := domain.NewMarker(
                ruleResult.Level,
                ruleResult.Type,
                fmt.Sprintf("Rule: %s", ruleResult.RuleID),
                0, 100,
                confidence,
                ruleResult.RuleID,
                "", "",
                false,
            )
            marker.AddMetadata("rule_expression", ruleResult.Expression)
            marker.AddMetadata("rule_explanation", ruleResult.Explanation)
            marker.AddMetadata("generation_timestamp", time.Now().Format(time.RFC3339))

            if markersList, exists := context["markers"].([]domain.Marker); exists && len(markersList) > 0 {
                marker.Start = markersList[0].Start
                marker.End = markersList[0].End
            }
            markers = append(markers, *marker)
        }
    }
    return markers
}

// calculateSemanticScore calculates an overall semantic score
func (se *SemanticEngine) calculateSemanticScore(
    entities []Entity,
    proximityAnalyses []ProximityAnalysis,
    ruleResults []RuleResult,
) float64 {
    if len(entities) == 0 && len(proximityAnalyses) == 0 && len(ruleResults) == 0 {
        return 0.0
    }

    var totalScore float64
    var weightCount float64

    if len(entities) > 0 {
        entityScore := 0.0
        for _, e := range entities {
            entityScore += e.Confidence
        }
        entityScore /= float64(len(entities))
        totalScore += entityScore * 0.4
        weightCount += 0.4
    }

    if len(proximityAnalyses) > 0 {
        proximityScore := 0.0
        for _, a := range proximityAnalyses {
            proximityScore += a.AverageRelatedness
        }
        proximityScore /= float64(len(proximityAnalyses))
        totalScore += proximityScore * 0.3
        weightCount += 0.3
    }

    if len(ruleResults) > 0 {
        ruleScore := 0.0
        triggeredCount := 0
        for _, r := range ruleResults {
            if r.Result {
                triggeredCount++
                ruleScore += r.Weight
            }
        }
        if triggeredCount > 0 {
            ruleScore /= float64(triggeredCount)
            ruleTriggerRatio := float64(triggeredCount) / float64(len(ruleResults))
            totalScore += (ruleScore * ruleTriggerRatio) * 0.3
            weightCount += 0.3
        }
    }

    if weightCount > 0 {
        return totalScore / weightCount
    }
    return totalScore
}

// addAuditEvent adds an audit event
func (se *SemanticEngine) addAuditEvent(event *domain.AuditEvent) {
    se.mu.Lock()
    defer se.mu.Unlock()
    se.auditEvents = append(se.auditEvents, *event)
}

// addAuditEvents adds multiple audit events
func (se *SemanticEngine) addAuditEvents(events []domain.AuditEvent) {
    se.mu.Lock()
    defer se.mu.Unlock()
    se.auditEvents = append(se.auditEvents, events...)
}

// GetAuditEvents returns all audit events
func (se *SemanticEngine) GetAuditEvents() []domain.AuditEvent {
    se.mu.RLock()
    defer se.mu.RUnlock()
    return se.auditEvents
}

// SemanticAnalysisResult represents the result of semantic analysis
type SemanticAnalysisResult struct {
    Timestamp         time.Time            `json:"timestamp"`
    TextHash          string               `json:"text_hash"`
    Entities          []Entity             `json:"entities"`
    ProximityAnalysis []ProximityAnalysis  `json:"proximity_analysis"`
    LogicalRules      []RuleResult         `json:"logical_rules"`
    GeneratedMarkers  []domain.Marker      `json:"generated_markers"`
    SemanticScore     float64              `json:"semantic_score"`
    AuditEvents       []domain.AuditEvent  `json:"audit_events,omitempty"`
}

// ProximityAnalysis represents proximity analysis results
type ProximityAnalysis struct {
    SourceID           string                 `json:"source_id"`
    SourceType         string                 `json:"source_type"`
    RelatedItems       []RelatedMarker        `json:"related_items"`
    AverageRelatedness float64                `json:"average_relatedness"`
    Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// Helper functions
func calculateAverageRelatedness(relatedItems []RelatedMarker) float64 {
    if len(relatedItems) == 0 {
        return 0.0
    }
    total := 0.0
    for _, item := range relatedItems {
        total += item.Relatedness
    }
    return total / float64(len(relatedItems))
}

func convertEntitiesToMarkers(entities []Entity) []domain.Marker {
    var markers []domain.Marker
    for _, entity := range entities {
        confidence := domain.ToFixedPoint(entity.Confidence) // правильный fixed-point масштаб
        marker := domain.NewMarker(
            3,
            entity.Type,
            entity.Text,
            entity.Start,
            entity.End,
            confidence,
            entity.SourceRuleID,
            "", "",
            true,
        )
        marker.ID = entity.ID
        markers = append(markers, *marker)
    }
    return markers
}