package composite_test

import (
    "testing"
    
    "semantic-analyzer/internal/domain"
    "semantic-analyzer/internal/engine/composite"
    "semantic-analyzer/pkg/config"
)

func TestBuilder_BuildContextWindows(t *testing.T) {
    // Create test configuration
    cfg := config.CompositeRulesConfig{
        ProximityWindow: 2,
        MinDensity:      0.1,
        MaxSpan:         100,
        MaxCandidates:   10,
        SelectionWeights: config.SelectionWeights{
            MarkerCount:   0.4,
            Density:       0.3,
            SpanCompactness: 0.2,
            TypeDiversity: 0.1,
        },
        TypeWeights: map[string]float64{
            "CONTACT":  1.0,
            "PRIORITY": 0.8,
        },
    }
    
    builder := composite.NewBuilder(cfg)
    
    // Create test atomic markers
    atomicMarkers := []domain.Marker{
        *domain.NewMarker(1, "CONTACT", "test@example.com", 10, 25, 0.9, "email_pattern", true),
        *domain.NewMarker(1, "PRIORITY", "urgent", 40, 46, 0.6, "important_terms", true),
        *domain.NewMarker(1, "CONTACT", "123-456-7890", 60, 73, 0.85, "phone_pattern", true),
    }
    
    // Create test sentences
    sentences := []string{
        "Contact us at test@example.com for assistance.",
        "This is an urgent matter.",
        "Call 123-456-7890 for immediate support.",
    }
    
    // Build context windows
    candidates := builder.BuildContextWindows(atomicMarkers, sentences)
    
    // Verify results
    if len(candidates) == 0 {
        t.Error("Expected at least one candidate, got none")
    }
    
    // Check that candidates have markers
    for i, candidate := range candidates {
        if candidate.MarkerCount < 2 {
            t.Errorf("Candidate %d should have at least 2 markers, got %d", i, candidate.MarkerCount)
        }
        
        if candidate.Density < cfg.MinDensity {
            t.Errorf("Candidate %d density %f below minimum %f", i, candidate.Density, cfg.MinDensity)
        }
        
        if candidate.SpanLength > cfg.MaxSpan {
            t.Errorf("Candidate %d span length %d exceeds maximum %d", i, candidate.SpanLength, cfg.MaxSpan)
        }
    }
}

func TestBuilder_MergeSimilarCandidates(t *testing.T) {
    cfg := config.CompositeRulesConfig{
        ProximityWindow: 2,
        MinDensity:      0.1,
        MaxSpan:         200,
        MaxCandidates:   10,
    }
    
    builder := composite.NewBuilder(cfg)
    
    // Create overlapping candidates
    candidates := []composite.CompositeCandidate{
        {
            ID:         "candidate1",
            SpanStart:  0,
            SpanEnd:    100,
            MarkerCount: 3,
        },
        {
            ID:         "candidate2",
            SpanStart:  50,
            SpanEnd:    150,
            MarkerCount: 2,
        },
        {
            ID:         "candidate3",
            SpanStart:  200,
            SpanEnd:    300,
            MarkerCount: 2,
        },
    }
    
    // Merge similar candidates
    merged := builder.MergeSimilarCandidates(candidates, 0.5)
    
    // Verify merging
    if len(merged) >= len(candidates) {
        t.Errorf("Expected fewer candidates after merging, got %d (was %d)", len(merged), len(candidates))
    }
    
    // Verify non-overlapping candidate remains separate
    foundNonOverlapping := false
    for _, candidate := range merged {
        if candidate.SpanStart == 200 && candidate.SpanEnd == 300 {
            foundNonOverlapping = true
            break
        }
    }
    
    if !foundNonOverlapping {
        t.Error("Non-overlapping candidate should remain separate")
    }
}