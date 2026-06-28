package semantic_test

import (
    "testing"
    
    "semantic-analyzer/internal/domain"
    "semantic-analyzer/internal/engine/semantic"
    "semantic-analyzer/pkg/config"
)

func TestProximityAnalyzer_CalculateSemanticRelatedness(t *testing.T) {
    // Create test configuration
    cfg := config.SemanticRulesConfig{
        Networks: map[string][]string{
            "technology": {"software", "hardware", "algorithm"},
            "business":   {"profit", "market", "strategy"},
        },
        ProximityWeights: config.ProximityWeights{
            NetworkMatch:   0.5,
            TypeSimilarity: 0.3,
            ContextOverlap: 0.2,
        },
    }
    
    analyzer := semantic.NewProximityAnalyzer(cfg)
    
    // Create test markers
    marker1 := domain.NewMarker(1, "TECH", "software development", 0, 18, 0.8, "rule1", true)
    marker2 := domain.NewMarker(1, "TECH", "hardware design", 20, 34, 0.7, "rule2", true)
    marker3 := domain.NewMarker(1, "BUSINESS", "profit margin", 40, 52, 0.6, "rule3", true)
    
    // Set context for markers
    marker1.Context = "We focus on software development and innovation."
    marker2.Context = "Our hardware design team creates advanced systems."
    marker3.Context = "The profit margin increased this quarter."
    
    // Calculate relatedness between tech markers
    relatedness1 := analyzer.CalculateSemanticRelatedness(*marker1, *marker2)
    
    if relatedness1 < 0.5 {
        t.Errorf("Expected high relatedness between tech markers, got %f", relatedness1)
    }
    
    // Calculate relatedness between tech and business markers
    relatedness2 := analyzer.CalculateSemanticRelatedness(*marker1, *marker3)
    
    if relatedness2 > 0.5 {
        t.Errorf("Expected low relatedness between different domain markers, got %f", relatedness2)
    }
}

func TestProximityAnalyzer_CalculateBatchRelatedness(t *testing.T) {
    cfg := config.SemanticRulesConfig{
        Networks: map[string][]string{
            "contact": {"email", "phone", "contact"},
            "action":  {"todo", "task", "action"},
        },
        ProximityWeights: config.ProximityWeights{
            NetworkMatch:   0.5,
            TypeSimilarity: 0.3,
            ContextOverlap: 0.2,
        },
    }
    
    analyzer := semantic.NewProximityAnalyzer(cfg)
    
    // Create test markers
    markers := []domain.Marker{
        *domain.NewMarker(1, "CONTACT", "test@example.com", 0, 16, 0.9, "email", true),
        *domain.NewMarker(1, "CONTACT", "123-456-7890", 20, 33, 0.85, "phone", true),
        *domain.NewMarker(1, "ACTION", "todo item", 40, 49, 0.6, "todo", true),
    }
    
    // Set context
    markers[0].Context = "Email: test@example.com for support"
    markers[1].Context = "Phone: 123-456-7890 for calls"
    markers[2].Context = "Todo item needs completion"
    
    // Calculate batch relatedness
    results := analyzer.CalculateBatchRelatedness(markers, 50)
    
    // Verify results
    if len(results) < 2 {
        t.Errorf("Expected at least 2 proximity results, got %d", len(results))
    }
    
    // Check that contact markers have high relatedness
    highRelatednessFound := false
    for _, result := range results {
        if result.Marker1ID == markers[0].ID && result.Marker2ID == markers[1].ID {
            if result.Relatedness > 0.7 {
                highRelatednessFound = true
            }
            break
        }
    }
    
    if !highRelatednessFound {
        t.Error("Expected high relatedness between contact markers")
    }
    
    // Results should be sorted by relatedness
    for i := 1; i < len(results); i++ {
        if results[i].Relatedness > results[i-1].Relatedness {
            t.Error("Results should be sorted by relatedness descending")
            break
        }
    }
}