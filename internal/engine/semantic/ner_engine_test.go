package semantic_test

import (
    "testing"
    
    "semantic-analyzer/internal/engine/semantic"
    "semantic-analyzer/pkg/config"
)

func TestNEREngine_ExtractEntities(t *testing.T) {
    // Create test configuration
    cfg := config.NERConfig{
        PersonTitles: []string{"Mr.", "Ms.", "Dr.", "Prof."},
        OrganizationSuffixes: []string{"Inc.", "Corp.", "Ltd."},
        LocationKeywords: []string{"street", "avenue", "city"},
        PersonPatterns: []config.RegexRuleConfig{
            {
                ID:      "person_name",
                Pattern: `\b(?:Mr\.|Ms\.|Dr\.|Prof\.)\s+[A-Z][a-z]+\s+[A-Z][a-z]+\b`,
                Weight:  0.9,
                Type:    "PERSON",
                Level:   3,
                Name:    "Person Name",
            },
        },
        OrganizationPatterns: []config.RegexRuleConfig{
            {
                ID:      "organization_name",
                Pattern: `\b[A-Z][A-Za-z0-9&\.\s]+(?:Inc\.|Corp\.|Ltd\.)\b`,
                Weight:  0.8,
                Type:    "ORGANIZATION",
                Level:   3,
                Name:    "Organization Name",
            },
        },
        LocationPatterns: []string{
            `\b[A-Z][a-z]+(?:[ -][A-Z][a-z]+)*\b`,
        },
    }
    
    engine := semantic.NewNEREngine(cfg)
    
    // Test text with various entities
    text := `Mr. John Smith works at Acme Corp. He lives on Main Street in New York City. 
             Contact Dr. Jane Doe at the University Hospital.`
    
    // Extract entities
    entities := engine.ExtractEntities(text)
    
    // Verify person entities
    personCount := 0
    for _, entity := range entities {
        if entity.Type == "PERSON" {
            personCount++
            if entity.Confidence < 0.5 {
                t.Errorf("Person entity confidence too low: %f", entity.Confidence)
            }
        }
    }
    
    if personCount < 2 {
        t.Errorf("Expected at least 2 person entities, got %d", personCount)
    }
    
    // Verify organization entities
    orgCount := 0
    for _, entity := range entities {
        if entity.Type == "ORGANIZATION" {
            orgCount++
        }
    }
    
    if orgCount == 0 {
        t.Error("Expected at least one organization entity")
    }
    
    // Verify location entities
    locationCount := 0
    for _, entity := range entities {
        if entity.Type == "LOCATION" {
            locationCount++
        }
    }
    
    if locationCount == 0 {
        t.Error("Expected at least one location entity")
    }
}

func TestNEREngine_RemoveOverlappingEntities(t *testing.T) {
    cfg := config.NERConfig{
        PersonTitles:        []string{"Mr."},
        OrganizationSuffixes: []string{"Inc."},
        LocationKeywords:    []string{"street"},
    }
    
    engine := semantic.NewNEREngine(cfg)
    
    // Create overlapping entities
    entities := []semantic.Entity{
        *semantic.NewEntity("PERSON", "John Smith", 0, 10, 0.9, "rule1"),
        *semantic.NewEntity("PERSON", "John Smith Doe", 0, 15, 0.8, "rule2"),
        *semantic.NewEntity("LOCATION", "Main Street", 20, 31, 0.7, "rule3"),
    }
    
    // Remove overlapping entities
    filtered := engine.RemoveOverlappingEntities(entities)
    
    // Verify overlapping entities are removed
    if len(filtered) >= len(entities) {
        t.Errorf("Expected fewer entities after removing overlaps, got %d (was %d)", 
            len(filtered), len(entities))
    }
    
    // Verify higher confidence entity is kept
    foundHighConfidence := false
    for _, entity := range filtered {
        if entity.Text == "John Smith" && entity.Confidence == 0.9 {
            foundHighConfidence = true
            break
        }
    }
    
    if !foundHighConfidence {
        t.Error("Higher confidence entity should be kept")
    }
}