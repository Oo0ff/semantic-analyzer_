package final

import (
    "fmt"
    "regexp"
    "strings"
    "sync"
    "time"

    "semantic-analyzer/internal/domain"
)

// FilterEngine handles all filtering operations
type FilterEngine struct {
    config        FilterConfig
    suppressionRegex map[string]*regexp.Regexp
    auditEvents   []domain.AuditEvent
    mu            sync.RWMutex
}

// NewFilterEngine creates a new filter engine
func NewFilterEngine(cfg FilterConfig) *FilterEngine {
    fe := &FilterEngine{
        config:          cfg,
        suppressionRegex: make(map[string]*regexp.Regexp),
        auditEvents:     []domain.AuditEvent{},
    }
    
    fe.initialize()
    return fe
}

// initialize sets up the filter engine
func (fe *FilterEngine) initialize() {
    fe.mu.Lock()
    defer fe.mu.Unlock()
    
    // Compile suppression regex patterns
    for _, rule := range fe.config.SuppressionRules {
        if rule.Enabled && rule.Type == "regex" {
            compiled, err := regexp.Compile(rule.Pattern)
            if err != nil {
                // Log error but continue
                fmt.Printf("Warning: Failed to compile suppression regex %s: %v\n", rule.ID, err)
                continue
            }
            fe.suppressionRegex[rule.ID] = compiled
        }
    }
}

// ApplySuppressionFilters applies suppression filters to markers
func (fe *FilterEngine) ApplySuppressionFilters(markers []domain.Marker) ([]domain.Marker, FilterResult) {
    startTime := time.Now()
    
    result := FilterResult{
        InputCount:  len(markers),
        Suppressed:  []SuppressedItem{},
        Statistics: FilterStatistics{
            TimeTakenMs: 0,
        },
    }
    
    // Create audit event
    auditEvent := domain.NewAuditEvent("filter_engine", "suppression_start")
    auditEvent.AddData("marker_count", len(markers))
    auditEvent.AddData("suppression_rules", len(fe.config.SuppressionRules))
    fe.addAuditEvent(auditEvent)
    
    var filtered []domain.Marker
    suppressedCount := 0
    
    for _, marker := range markers {
        shouldSuppress := false
        suppressionReason := ""
        suppressionRuleID := ""
        
        // Check each suppression rule
        for _, rule := range fe.config.SuppressionRules {
            if !rule.Enabled {
                continue
            }
            
            // Check if rule applies to this marker type
            applies := false
            for _, markerType := range rule.ApplyTo {
                if markerType == "*" || marker.Type == markerType {
                    applies = true
                    break
                }
            }
            
            if !applies {
                continue
            }
            
            // Apply suppression logic based on rule type
            switch rule.Type {
            case "regex":
                if regex, exists := fe.suppressionRegex[rule.ID]; exists {
                    if regex.MatchString(marker.TextSpan) {
                        shouldSuppress = true
                        suppressionReason = rule.Reason
                        suppressionRuleID = rule.ID
                        break
                    }
                }
            case "keyword":
                lowerText := strings.ToLower(marker.TextSpan)
                lowerPattern := strings.ToLower(rule.Pattern)
                if strings.Contains(lowerText, lowerPattern) {
                    shouldSuppress = true
                    suppressionReason = rule.Reason
                    suppressionRuleID = rule.ID
                    break
                }
            case "pattern":
                // Simple pattern matching (wildcard support)
                if fe.matchesPattern(marker.TextSpan, rule.Pattern) {
                    shouldSuppress = true
                    suppressionReason = rule.Reason
                    suppressionRuleID = rule.ID
                    break
                }
            }
            
            if shouldSuppress {
                break
            }
        }
        
        if shouldSuppress {
            suppressedCount++
            result.Suppressed = append(result.Suppressed, SuppressedItem{
                MarkerID: marker.ID,
                RuleID:   suppressionRuleID,
                Reason:   suppressionReason,
                Type:     marker.Type,
            })
            
            // Add audit event for suppression
            suppressEvent := domain.NewAuditEvent("filter_engine", "marker_suppressed")
            suppressEvent.MarkerID = marker.ID
            suppressEvent.RuleID = suppressionRuleID
            suppressEvent.AddData("reason", suppressionReason)
            suppressEvent.AddData("text", marker.TextSpan)
            suppressEvent.AddData("type", marker.Type)
            fe.addAuditEvent(suppressEvent)
        } else {
            filtered = append(filtered, marker)
        }
    }
    
    result.OutputCount = len(filtered)
    result.Statistics.SuppressionCount = suppressedCount
    result.Statistics.TimeTakenMs = float64(time.Since(startTime).Milliseconds())
    
    // Create completion audit event
    endEvent := domain.NewAuditEvent("filter_engine", "suppression_complete")
    endEvent.AddData("input_markers", len(markers))
    endEvent.AddData("output_markers", len(filtered))
    endEvent.AddData("suppressed", suppressedCount)
    endEvent.SetDuration(startTime)
    fe.addAuditEvent(endEvent)
    
    return filtered, result
}

// DeduplicateOverlapping removes duplicate or overlapping markers
func (fe *FilterEngine) DeduplicateOverlapping(markers []domain.Marker) ([]domain.Marker, FilterResult) {
    if !fe.config.Deduplication.Enabled {
        return markers, FilterResult{
            InputCount:  len(markers),
            OutputCount: len(markers),
            Statistics:  FilterStatistics{},
        }
    }
    
    startTime := time.Now()
    
    result := FilterResult{
        InputCount:    len(markers),
        Deduplicated:  []DeduplicatedItem{},
        Statistics: FilterStatistics{
            TimeTakenMs: 0,
        },
    }
    
    auditEvent := domain.NewAuditEvent("filter_engine", "deduplication_start")
    auditEvent.AddData("marker_count", len(markers))
    auditEvent.AddData("strategy", fe.config.Deduplication.Strategy)
    fe.addAuditEvent(auditEvent)
    
    // Sort markers by start position for easier comparison
    sortedMarkers := make([]domain.Marker, len(markers))
    copy(sortedMarkers, markers)
    fe.sortMarkersByPosition(sortedMarkers)
    
    var deduplicated []domain.Marker
    removedCount := 0
    
    for i := 0; i < len(sortedMarkers); i++ {
        current := sortedMarkers[i]
        keepCurrent := true
        
        // Compare with already kept markers
        for j, kept := range deduplicated {
            if current.Overlaps(&kept) {
                overlapRatio := fe.calculateOverlapRatio(&current, &kept)
                
                if overlapRatio >= fe.config.Deduplication.OverlapThreshold {
                    // Decide which marker to keep based on strategy
                    keepCurrent = fe.shouldKeepMarker(&current, &kept, overlapRatio)
                    
                    if !keepCurrent {
                        removedCount++
                        result.Deduplicated = append(result.Deduplicated, DeduplicatedItem{
                            KeptMarkerID:   kept.ID,
                            RemovedMarkerID: current.ID,
                            Reason:         "overlap",
                            OverlapRatio:   overlapRatio,
                        })
                        
                        // Add audit event
                        dedupEvent := domain.NewAuditEvent("filter_engine", "marker_deduplicated")
                        dedupEvent.MarkerID = current.ID
                        dedupEvent.AddData("kept_marker_id", kept.ID)
                        dedupEvent.AddData("overlap_ratio", overlapRatio)
                        dedupEvent.AddData("strategy", fe.config.Deduplication.Strategy)
                        fe.addAuditEvent(dedupEvent)
                        
                        break
                    } else {
                        // Remove the previously kept marker
                        result.Deduplicated = append(result.Deduplicated, DeduplicatedItem{
                            KeptMarkerID:   current.ID,
                            RemovedMarkerID: kept.ID,
                            Reason:         "overlap",
                            OverlapRatio:   overlapRatio,
                        })
                        
                        // Remove the kept marker from the list
                        deduplicated = append(deduplicated[:j], deduplicated[j+1:]...)
                        
                        // Add audit event
                        dedupEvent := domain.NewAuditEvent("filter_engine", "marker_deduplicated")
                        dedupEvent.MarkerID = kept.ID
                        dedupEvent.AddData("kept_marker_id", current.ID)
                        dedupEvent.AddData("overlap_ratio", overlapRatio)
                        dedupEvent.AddData("strategy", fe.config.Deduplication.Strategy)
                        fe.addAuditEvent(dedupEvent)
                        
                        // Continue checking with updated list
                        i-- // Recheck current against remaining kept markers
                        break
                    }
                }
            }
        }
        
        if keepCurrent {
            deduplicated = append(deduplicated, current)
        }
    }
    
    result.OutputCount = len(deduplicated)
    result.Statistics.DeduplicationCount = removedCount
    result.Statistics.TimeTakenMs = float64(time.Since(startTime).Milliseconds())
    
    endEvent := domain.NewAuditEvent("filter_engine", "deduplication_complete")
    endEvent.AddData("input_markers", len(markers))
    endEvent.AddData("output_markers", len(deduplicated))
    endEvent.AddData("removed", removedCount)
    endEvent.SetDuration(startTime)
    fe.addAuditEvent(endEvent)
    
    return deduplicated, result
}

// matchesPattern checks if text matches a pattern with wildcards
func (fe *FilterEngine) matchesPattern(text, pattern string) bool {
    // Convert wildcard pattern to regex
    regexPattern := "^" + regexp.QuoteMeta(pattern)
    regexPattern = strings.ReplaceAll(regexPattern, "\\*", ".*")
    regexPattern = strings.ReplaceAll(regexPattern, "\\?", ".")
    regexPattern += "$"
    
    regex, err := regexp.Compile(regexPattern)
    if err != nil {
        return false
    }
    
    return regex.MatchString(text)
}

// calculateOverlapRatio calculates the overlap ratio between two markers
func (fe *FilterEngine) calculateOverlapRatio(m1, m2 *domain.Marker) float64 {
    overlapStart := max(m1.Start, m2.Start)
    overlapEnd := min(m1.End, m2.End)
    
    if overlapEnd <= overlapStart {
        return 0.0
    }
    
    overlapLength := overlapEnd - overlapStart
    m1Length := m1.End - m1.Start
    m2Length := m2.End - m2.Start
    
    // Return the maximum overlap ratio relative to either marker
    ratio1 := float64(overlapLength) / float64(m1Length)
    ratio2 := float64(overlapLength) / float64(m2Length)
    
    if ratio1 > ratio2 {
        return ratio1
    }
    return ratio2
}

// shouldKeepMarker decides which marker to keep during deduplication
func (fe *FilterEngine) shouldKeepMarker(m1, m2 *domain.Marker, overlapRatio float64) bool {
    // Convert min confidence delta to fixed-point int64
    delta := domain.ToFixedPoint(fe.config.Deduplication.MinConfidenceDelta)

    switch fe.config.Deduplication.Strategy {
    case "keep_highest":
        // Keep marker with higher confidence (int64 comparison)
        if m1.Confidence > m2.Confidence + delta {
            return true
        } else if m2.Confidence > m1.Confidence + delta {
            return false
        }
        // If confidence difference is small, fall through to other criteria
        
    case "keep_earliest":
        // Keep marker that appears earlier in text
        if m1.Start < m2.Start {
            return true
        }
        return false
        
    case "merge":
        // In a full implementation, this would merge markers
        // For now, default to keeping the higher confidence one
        if m1.Confidence >= m2.Confidence {
            return true
        }
        return false
    }
    
    // Default: keep m1 if it has higher level
    if m1.Level > m2.Level {
        return true
    } else if m2.Level > m1.Level {
        return false
    }
    
    // If levels are equal, keep the one with higher confidence
    return m1.Confidence >= m2.Confidence
}

// sortMarkersByPosition sorts markers by start position
func (fe *FilterEngine) sortMarkersByPosition(markers []domain.Marker) {
    for i := 0; i < len(markers); i++ {
        for j := i + 1; j < len(markers); j++ {
            if markers[i].Start > markers[j].Start ||
               (markers[i].Start == markers[j].Start && markers[i].End > markers[j].End) {
                markers[i], markers[j] = markers[j], markers[i]
            }
        }
    }
}

// addAuditEvent adds an audit event
func (fe *FilterEngine) addAuditEvent(event *domain.AuditEvent) {
    fe.mu.Lock()
    defer fe.mu.Unlock()
    fe.auditEvents = append(fe.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (fe *FilterEngine) GetAuditEvents() []domain.AuditEvent {
    fe.mu.RLock()
    defer fe.mu.RUnlock()
    return fe.auditEvents
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