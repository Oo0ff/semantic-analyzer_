package final

import (
    "fmt"
    "sort"
    "time"

    "semantic-analyzer/internal/domain"
    "semantic-analyzer/pkg/determinism"
)

// ConflictResolver handles conflict resolution between markers
type ConflictResolver struct {
    config      ConflictResolutionConfig
    seedManager *determinism.SeedManager
    auditEvents []domain.AuditEvent
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(cfg ConflictResolutionConfig, seedManager *determinism.SeedManager) *ConflictResolver {
    return &ConflictResolver{
        config:      cfg,
        seedManager: seedManager,
        auditEvents: []domain.AuditEvent{},
    }
}

// ResolveConflicts resolves conflicts between markers
func (cr *ConflictResolver) ResolveConflicts(markers []domain.Marker) ([]domain.Marker, FilterResult) {
    if !cr.config.Enabled {
        return markers, FilterResult{
            InputCount:  len(markers),
            OutputCount: len(markers),
            Statistics:  FilterStatistics{},
        }
    }
    
    startTime := time.Now()
    
    result := FilterResult{
        InputCount: len(markers),
        Conflicts:  []Conflict{},
        Resolved:   []ResolvedItem{},
        Statistics: FilterStatistics{
            TimeTakenMs: 0,
        },
    }
    
    // Create audit event
    auditEvent := domain.NewAuditEvent("conflict_resolver", "resolve_conflicts_start")
    auditEvent.AddData("marker_count", len(markers))
    auditEvent.AddData("priorities", cr.config.Priorities)
    cr.addAuditEvent(auditEvent)
    
    // Detect conflicts
    conflicts := cr.detectConflicts(markers)
    result.Conflicts = conflicts
    result.Statistics.ConflictCount = len(conflicts)
    
    // Resolve conflicts
    resolvedMarkers, resolvedItems := cr.resolveDetectedConflicts(markers, conflicts)
    result.Resolved = resolvedItems
    result.OutputCount = len(resolvedMarkers)
    result.Statistics.TimeTakenMs = float64(time.Since(startTime).Milliseconds())
    
    // Create completion audit event
    endEvent := domain.NewAuditEvent("conflict_resolver", "conflicts_resolved")
    endEvent.AddData("input_markers", len(markers))
    endEvent.AddData("output_markers", len(resolvedMarkers))
    endEvent.AddData("conflicts_detected", len(conflicts))
    endEvent.AddData("conflicts_resolved", len(resolvedItems))
    endEvent.SetDuration(startTime)
    cr.addAuditEvent(endEvent)
    
    return resolvedMarkers, result
}

// detectConflicts detects conflicts between markers
func (cr *ConflictResolver) detectConflicts(markers []domain.Marker) []Conflict {
    var conflicts []Conflict
    
    // Sort markers by position for efficient comparison
    sortedMarkers := make([]domain.Marker, len(markers))
    copy(sortedMarkers, markers)
    cr.sortMarkersByPosition(sortedMarkers)
    
    for i := 0; i < len(sortedMarkers); i++ {
        for j := i + 1; j < len(sortedMarkers); j++ {
            conflict := cr.detectConflict(&sortedMarkers[i], &sortedMarkers[j])
            if conflict != nil {
                conflicts = append(conflicts, *conflict)
            }
        }
    }
    
    return conflicts
}

// detectConflict detects if there's a conflict between two markers
func (cr *ConflictResolver) detectConflict(m1, m2 *domain.Marker) *Conflict {
    // Check for overlap
    if m1.Overlaps(m2) {
        overlapRatio := cr.calculateOverlapRatio(m1, m2)
        conflictType := "overlap"
        severity := overlapRatio
        
        // Check for contradiction (different types in same location)
        if m1.Type != m2.Type && overlapRatio > 0.8 {
            conflictType = "contradiction"
            severity = 1.0
        }
        
        // Check for redundancy (same type, high overlap)
        if m1.Type == m2.Type && overlapRatio > 0.9 {
            conflictType = "redundancy"
            severity = 0.9
        }
        
        return &Conflict{
            Marker1:      *m1,
            Marker2:      *m2,
            Type:         conflictType,
            OverlapRatio: overlapRatio,
            Severity:     severity,
        }
    }
    
    return nil
}

// resolveDetectedConflicts resolves detected conflicts
func (cr *ConflictResolver) resolveDetectedConflicts(markers []domain.Marker, conflicts []Conflict) ([]domain.Marker, []ResolvedItem) {
    // Create a map of markers for easy access
    markerMap := make(map[string]*domain.Marker)
    for i := range markers {
        markerMap[markers[i].ID] = &markers[i]
    }
    
    // Track which markers to remove
    toRemove := make(map[string]bool)
    var resolvedItems []ResolvedItem
    
    // Process conflicts in order of severity
    sortedConflicts := make([]Conflict, len(conflicts))
    copy(sortedConflicts, conflicts)
    sort.Slice(sortedConflicts, func(i, j int) bool {
        return sortedConflicts[i].Severity > sortedConflicts[j].Severity
    })
    
    for _, conflict := range sortedConflicts {
        m1 := markerMap[conflict.Marker1.ID]
        m2 := markerMap[conflict.Marker2.ID]
        
        // Skip if either marker is already marked for removal
        if toRemove[m1.ID] || toRemove[m2.ID] {
            continue
        }
        
        // Determine winner based on priority rules
        winner, loser, reason := cr.determineWinner(m1, m2)
        
        if winner != nil && loser != nil {
            // Mark loser for removal
            toRemove[loser.ID] = true
            
            // Record resolution
            resolvedItems = append(resolvedItems, ResolvedItem{
                ConflictID:  fmt.Sprintf("conflict_%s_%s", m1.ID, m2.ID),
                WinnerID:    winner.ID,
                LoserID:     loser.ID,
                Reason:      reason,
                RuleApplied: cr.getAppliedRule(m1, m2),
            })
            
            // Update conflict with resolution
            conflict.Resolution = "resolved"
            conflict.WinnerID = winner.ID
            conflict.Reason = reason
            
            // Add audit event
            resolveEvent := domain.NewAuditEvent("conflict_resolver", "conflict_resolved")
            resolveEvent.AddData("conflict_type", conflict.Type)
            resolveEvent.AddData("winner_id", winner.ID)
            resolveEvent.AddData("loser_id", loser.ID)
            resolveEvent.AddData("reason", reason)
            resolveEvent.AddData("overlap_ratio", conflict.OverlapRatio)
            cr.addAuditEvent(resolveEvent)
        }
    }
    
    // Build result list excluding removed markers
    var resolved []domain.Marker
    for _, marker := range markers {
        if !toRemove[marker.ID] {
            resolved = append(resolved, marker)
        }
    }
    
    return resolved, resolvedItems
}

// determineWinner determines which marker should win in a conflict
func (cr *ConflictResolver) determineWinner(m1, m2 *domain.Marker) (*domain.Marker, *domain.Marker, string) {
    // Apply priority rules in order
    for _, priority := range cr.config.Priorities {
        switch priority {
        case "level":
            if m1.Level > m2.Level {
                return m1, m2, "higher level"
            } else if m2.Level > m1.Level {
                return m2, m1, "higher level"
            }
            
        case "confidence":
            confidenceDelta := m1.Confidence - m2.Confidence
            if confidenceDelta > 50000 { // Significant difference (0.05 * FixedPointScale)
                return m1, m2, "higher confidence"
            } else if confidenceDelta < -50000 {
                return m2, m1, "higher confidence"
            }
            
        case "rule_priority":
            // Compare rule priorities (simplified - in real implementation, would look up rule priorities)
            if m1.RuleID < m2.RuleID { // Simplified comparison
                return m1, m2, "higher rule priority"
            } else if m2.RuleID < m1.RuleID {
                return m2, m1, "higher rule priority"
            }
            
        case "specificity":
            // More specific marker (shorter span) wins
            specificity1 := float64(m1.End-m1.Start)
            specificity2 := float64(m2.End-m2.Start)
            if specificity1 < specificity2 {
                return m1, m2, "more specific"
            } else if specificity2 < specificity1 {
                return m2, m1, "more specific"
            }
        }
    }
    
    // If all else fails, use tie-breaker
    return cr.applyTieBreaker(m1, m2)
}

// applyTieBreaker applies tie-breaking logic
func (cr *ConflictResolver) applyTieBreaker(m1, m2 *domain.Marker) (*domain.Marker, *domain.Marker, string) {
    switch cr.config.TieBreaker {
    case "random_seed":
        // Use deterministic random based on marker IDs
        randVal := cr.seedManager.GetDeterministicRandom(fmt.Sprintf("tie_%s_%s", m1.ID, m2.ID))
        if randVal < 0.5 {
            return m1, m2, "random selection (tie-breaker)"
        }
        return m2, m1, "random selection (tie-breaker)"
        
    case "position":
        // Earlier marker wins
        if m1.Start < m2.Start {
            return m1, m2, "earlier position (tie-breaker)"
        }
        return m2, m1, "earlier position (tie-breaker)"
        
    case "rule_id":
        // Lower rule ID wins (simplified)
        if m1.RuleID < m2.RuleID {
            return m1, m2, "lower rule ID (tie-breaker)"
        }
        return m2, m1, "lower rule ID (tie-breaker)"
        
    default:
        // Default: keep first marker
        return m1, m2, "default (first marker)"
    }
}

// getAppliedRule returns which rule was applied
func (cr *ConflictResolver) getAppliedRule(m1, m2 *domain.Marker) string {
    // This would normally look up which priority rule decided the conflict
    // For simplicity, return a generic description
    return fmt.Sprintf("Priority rules: %v", cr.config.Priorities)
}

// calculateOverlapRatio calculates overlap ratio between two markers
func (cr *ConflictResolver) calculateOverlapRatio(m1, m2 *domain.Marker) float64 {
    overlapStart := max(m1.Start, m2.Start)
    overlapEnd := min(m1.End, m2.End)
    
    if overlapEnd <= overlapStart {
        return 0.0
    }
    
    overlapLength := overlapEnd - overlapStart
    m1Length := m1.End - m1.Start
    
    return float64(overlapLength) / float64(m1Length)
}

// sortMarkersByPosition sorts markers by start position
func (cr *ConflictResolver) sortMarkersByPosition(markers []domain.Marker) {
    sort.Slice(markers, func(i, j int) bool {
        if markers[i].Start == markers[j].Start {
            return markers[i].End < markers[j].End
        }
        return markers[i].Start < markers[j].Start
    })
}

// addAuditEvent adds an audit event
func (cr *ConflictResolver) addAuditEvent(event *domain.AuditEvent) {
    cr.auditEvents = append(cr.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (cr *ConflictResolver) GetAuditEvents() []domain.AuditEvent {
    return cr.auditEvents
}