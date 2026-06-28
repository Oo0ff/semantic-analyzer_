package audit

import (
    "math"
    "time"

    "semantic-analyzer/internal/domain"
)

// ConfidenceScorer calculates final confidence scores
type ConfidenceScorer struct {
    config      ConfidenceConfig
    auditLogger *Logger
}

// ConfidenceConfig holds confidence scoring configuration
type ConfidenceConfig struct {
    BaseWeights      map[string]float64 `json:"base_weights" yaml:"base_weights"`
    RuleCertainty    map[string]float64 `json:"rule_certainty" yaml:"rule_certainty"`
    DecayRate        float64            `json:"decay_rate" yaml:"decay_rate"`
    MinConfidence    float64            `json:"min_confidence" yaml:"min_confidence"`
    MaxConfidence    float64            `json:"max_confidence" yaml:"max_confidence"`
    ConflictPenalty  float64            `json:"conflict_penalty" yaml:"conflict_penalty"`
    SupportBonus     float64            `json:"support_bonus" yaml:"support_bonus"`
}

// NewConfidenceScorer creates a new confidence scorer
func NewConfidenceScorer(cfg ConfidenceConfig, auditLogger *Logger) *ConfidenceScorer {
    return &ConfidenceScorer{
        config:      cfg,
        auditLogger: auditLogger,
    }
}

// CalculateFinalConfidence calculates the final confidence score for a marker
func (cs *ConfidenceScorer) CalculateFinalConfidence(marker domain.Marker, auditTrace []domain.AuditEvent) float64 {
    startTime := time.Now()
    
    // Log start of confidence calculation
    event := cs.auditLogger.LogWithParent("confidence_scorer", "calculate_start", "")
    event.MarkerID = marker.ID
    event.AddData("marker_type", marker.Type)
    event.AddData("initial_confidence", marker.Confidence)
    
    // Calculate base score from marker confidence
    baseScore := marker.Confidence
    
    // Adjust for rule certainty
    ruleCertainty := cs.getRuleCertainty(marker.RuleID)
    ruleAdjusted := baseScore * ruleCertainty
    
    // Adjust for conflict history
    conflictAdjustment := cs.calculateConflictAdjustment(marker, auditTrace)
    
    // Adjust for supporting evidence
    supportAdjustment := cs.calculateSupportAdjustment(marker, auditTrace)
    
    // Adjust for depth of analysis
    depthAdjustment := cs.calculateDepthAdjustment(marker, auditTrace)
    
    // Apply time decay if applicable
    timeDecay := cs.calculateTimeDecay(marker, auditTrace)
    
    // Calculate final confidence
    finalConfidence := ruleAdjusted + conflictAdjustment + supportAdjustment + depthAdjustment
    finalConfidence *= timeDecay
    
    // Apply bounds
    finalConfidence = math.Max(finalConfidence, cs.config.MinConfidence)
    finalConfidence = math.Min(finalConfidence, cs.config.MaxConfidence)
    
    // Round to 2 decimal places
    finalConfidence = math.Round(finalConfidence*100) / 100
    
    // Log completion
    cs.auditLogger.LogDuration(event.ID, startTime)
    
    endEvent := cs.auditLogger.LogWithParent("confidence_scorer", "calculate_complete", event.ID)
    endEvent.MarkerID = marker.ID
    endEvent.AddData("final_confidence", finalConfidence)
    endEvent.AddData("components", map[string]float64{
        "base_score":          baseScore,
        "rule_adjusted":       ruleAdjusted,
        "conflict_adjustment": conflictAdjustment,
        "support_adjustment":  supportAdjustment,
        "depth_adjustment":    depthAdjustment,
        "time_decay":          timeDecay,
    })
    
    return finalConfidence
}

// getRuleCertainty gets the certainty factor for a rule
func (cs *ConfidenceScorer) getRuleCertainty(ruleID string) float64 {
    if certainty, exists := cs.config.RuleCertainty[ruleID]; exists {
        return certainty
    }
    
    // Default certainty based on rule type
    if len(ruleID) > 0 {
        // Check for rule type patterns
        if contains(ruleID, "regex") {
            return 0.9
        } else if contains(ruleID, "keyword") {
            return 0.7
        } else if contains(ruleID, "pattern") {
            return 0.8
        }
    }
    
    return 0.6 // Default certainty
}

// calculateConflictAdjustment calculates adjustment based on conflict history
func (cs *ConfidenceScorer) calculateConflictAdjustment(marker domain.Marker, auditTrace []domain.AuditEvent) float64 {
    conflictCount := 0
    totalSeverity := 0.0
    
    for _, event := range auditTrace {
        if event.Stage == "conflict_resolver" && event.Action == "conflict_resolved" {
            if data, ok := event.DataSnapshot.(map[string]interface{}); ok {
                if loserID, exists := data["loser_id"].(string); exists && loserID == marker.ID {
                    conflictCount++
                    if severity, exists := data["severity"].(float64); exists {
                        totalSeverity += severity
                    }
                }
            }
        }
    }
    
    if conflictCount == 0 {
        return 0.0
    }
    
    averageSeverity := totalSeverity / float64(conflictCount)
    penalty := -averageSeverity * cs.config.ConflictPenalty * float64(conflictCount)
    
    return penalty
}

// calculateSupportAdjustment calculates adjustment based on supporting evidence
func (cs *ConfidenceScorer) calculateSupportAdjustment(marker domain.Marker, auditTrace []domain.AuditEvent) float64 {
    supportCount := 0
    totalSupportStrength := 0.0
    
    for _, event := range auditTrace {
        if event.Stage == "semantic_analysis" {
            if event.Action == "proximity_related" || event.Action == "logical_match" {
                if data, ok := event.DataSnapshot.(map[string]interface{}); ok {
                    if markerID, exists := data["marker_id"].(string); exists && markerID == marker.ID {
                        supportCount++
                        if strength, exists := data["strength"].(float64); exists {
                            totalSupportStrength += strength
                        }
                    }
                }
            }
        }
    }
    
    if supportCount == 0 {
        return 0.0
    }
    
    averageStrength := totalSupportStrength / float64(supportCount)
    bonus := averageStrength * cs.config.SupportBonus * float64(supportCount)
    
    return bonus
}

// calculateDepthAdjustment calculates adjustment based on depth of analysis
func (cs *ConfidenceScorer) calculateDepthAdjustment(marker domain.Marker, auditTrace []domain.AuditEvent) float64 {
    // Count analysis phases the marker passed through
    phases := make(map[string]bool)
    
    for _, event := range auditTrace {
        if event.MarkerID == marker.ID {
            phases[event.Stage] = true
        }
    }
    
    phaseCount := len(phases)
    
    // Higher level markers go through more phases
    levelBonus := float64(marker.Level) * 0.05
    
    // Phase count bonus
    phaseBonus := float64(phaseCount) * 0.03
    
    return levelBonus + phaseBonus
}

// calculateTimeDecay calculates time decay factor
func (cs *ConfidenceScorer) calculateTimeDecay(marker domain.Marker, auditTrace []domain.AuditEvent) float64 {
    if cs.config.DecayRate <= 0 {
        return 1.0
    }
    
    // Find the earliest relevant audit event for this marker
    var earliestTime *time.Time
    
    for _, event := range auditTrace {
        if event.MarkerID == marker.ID && event.Timestamp.After(marker.DetectedAt) {
            if earliestTime == nil || event.Timestamp.Before(*earliestTime) {
                t := event.Timestamp
                earliestTime = &t
            }
        }
    }
    
    if earliestTime == nil {
        return 1.0
    }
    
    // Calculate decay based on time since first analysis
    elapsed := time.Since(*earliestTime).Hours()
    decay := math.Exp(-cs.config.DecayRate * elapsed)
    
    return decay
}

// ScoreAllMarkers calculates final confidence scores for all markers
func (cs *ConfidenceScorer) ScoreAllMarkers(markers []domain.Marker, auditTrail *domain.AuditTrail) []domain.Marker {
    startTime := time.Now()
    
    event := cs.auditLogger.LogWithParent("confidence_scorer", "score_all_start", "")
    event.AddData("marker_count", len(markers))
    
    // Convert audit trail to slice for easier processing
    var auditEvents []domain.AuditEvent
    if auditTrail != nil {
        for _, event := range auditTrail.Events {
            auditEvents = append(auditEvents, *event)
        }
    }
    
    scoredMarkers := make([]domain.Marker, len(markers))
    
    for i, marker := range markers {
        scoredMarker := marker
        finalConfidence := cs.CalculateFinalConfidence(marker, auditEvents)
        scoredMarker.Confidence = finalConfidence
        scoredMarker.Score = finalConfidence // Update score as well
        
        // Add confidence metadata
        scoredMarker.AddMetadata("final_confidence_calculated", true)
        scoredMarker.AddMetadata("confidence_timestamp", time.Now().Format(time.RFC3339))
        
        scoredMarkers[i] = scoredMarker
    }
    
    // Calculate statistics
    stats := cs.calculateConfidenceStatistics(scoredMarkers)
    
    cs.auditLogger.LogDuration(event.ID, startTime)
    
    endEvent := cs.auditLogger.LogWithParent("confidence_scorer", "score_all_complete", event.ID)
    endEvent.AddData("processed_count", len(scoredMarkers))
    endEvent.AddData("statistics", stats)
    
    return scoredMarkers
}

// calculateConfidenceStatistics calculates confidence statistics
func (cs *ConfidenceScorer) calculateConfidenceStatistics(markers []domain.Marker) map[string]interface{} {
    if len(markers) == 0 {
        return map[string]interface{}{
            "count":     0,
            "average":   0.0,
            "min":       0.0,
            "max":       0.0,
            "std_dev":   0.0,
        }
    }
    
    total := 0.0
    min := 1.0
    max := 0.0
    countByLevel := make(map[int]int)
    sumByLevel := make(map[int]float64)
    
    for _, marker := range markers {
        confidence := marker.Confidence
        total += confidence
        
        if confidence < min {
            min = confidence
        }
        if confidence > max {
            max = confidence
        }
        
        countByLevel[marker.Level]++
        sumByLevel[marker.Level] += confidence
    }
    
    average := total / float64(len(markers))
    
    // Calculate standard deviation
    varianceSum := 0.0
    for _, marker := range markers {
        diff := marker.Confidence - average
        varianceSum += diff * diff
    }
    stdDev := math.Sqrt(varianceSum / float64(len(markers)))
    
    // Calculate averages by level
    avgByLevel := make(map[int]float64)
    for level, count := range countByLevel {
        avgByLevel[level] = sumByLevel[level] / float64(count)
    }
    
    return map[string]interface{}{
        "count":         len(markers),
        "average":       average,
        "min":           min,
        "max":           max,
        "std_dev":       stdDev,
        "average_by_level": avgByLevel,
    }
}

// Helper function
func contains(s, substr string) bool {
    return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
           (s[:len(substr)] == substr || contains(s[1:], substr)))
}