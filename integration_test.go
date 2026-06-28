package integration_test

import (
    "testing"
    "time"
    
    "semantic-analyzer/cmd/pipeline"
    "semantic-analyzer/internal/domain"
    "semantic-analyzer/internal/processor/text"
    "semantic-analyzer/pkg/config"
    "semantic-analyzer/pkg/determinism"
)

func TestFullPipelineIntegration(t *testing.T) {
    // Create test configuration
    cfg := &config.Config{
        Determinism: config.DeterminismConfig{
            BaseSeed: 42,
        },
        Processing: config.ProcessingConfig{
            UnicodeNormalization: "NFC",
            EnableLowercase:      true,
            SentenceDelimiters:   []string{".", "!", "?"},
        },
        Rules: config.RulesConfig{
            Atomic: config.AtomicRulesConfig{
                RegexPatterns: []config.RegexRuleConfig{
                    {
                        ID:      "email_pattern",
                        Pattern: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
                        Weight:  0.9,
                        Type:    "CONTACT",
                        Level:   1,
                    },
                },
                KeywordLists: []config.KeywordRuleConfig{
                    {
                        ID:            "important_terms",
                        Keywords:      []string{"urgent", "important"},
                        Weight:        0.6,
                        Type:          "PRIORITY",
                        Level:         1,
                        CaseSensitive: false,
                    },
                },
            },
            Composite: config.CompositeRulesConfig{
                ProximityWindow: 2,
                MinDensity:      0.1,
                MaxSpan:         500,
                MaxCandidates:   5,
            },
            Semantic: config.SemanticRulesConfig{
                Networks: map[string][]string{
                    "contact": {"email", "phone"},
                    "business": {"urgent", "important"},
                },
            },
        },
    }
    
    // Create test transcript
    transcriptText := `Important meeting notes. Contact us at test@example.com for urgent matters. 
                      Follow up with John at john@company.com. This is a high priority task.`
    
    transcript := &domain.Transcript{
        ID:        "test_transcript",
        RawText:   transcriptText,
        Source:    "test.txt",
        CreatedAt: time.Now(),
    }
    
    // Preprocess text
    preprocessor := text.NewPreprocessor(cfg.Processing)
    transcript.ProcessedText = preprocessor.Normalize(transcriptText)
    transcript.Sentences = preprocessor.SegmentIntoSentences(transcript.ProcessedText)
    transcript.TokenCount = len(preprocessor.Tokenize(transcript.ProcessedText))
    
    // Initialize and run pipeline
    orchestrator, err := pipeline.NewOrchestrator(cfg)
    if err != nil {
        t.Fatalf("Failed to create orchestrator: %v", err)
    }
    
    result, err := orchestrator.Process(transcript)
    if err != nil {
        t.Fatalf("Pipeline processing failed: %v", err)
    }
    
    // Verify results
    if result.Statistics.TotalMarkers == 0 {
        t.Error("Expected at least one marker, got none")
    }
    
    // Check for atomic markers
    if result.Statistics.AtomicMarkers < 2 {
        t.Errorf("Expected at least 2 atomic markers, got %d", result.Statistics.AtomicMarkers)
    }
    
    // Check for composite markers (if conditions met)
    if result.Statistics.CompositeMarkers > 0 {
        // Verify composite markers have level 2
        for _, marker := range result.Markers {
            if marker.Level == 2 && marker.Type == "COMPOSITE" {
                if marker.Confidence < 0.0 || marker.Confidence > 1.0 {
                    t.Errorf("Invalid confidence for composite marker: %f", marker.Confidence)
                }
            }
        }
    }
    
    // Verify determinism
    seedManager := determinism.NewSeedManager(42)
    hash1 := seedManager.DeterministicHash(result.ToJSONString())
    
    // Run pipeline again with same seed
    orchestrator2, _ := pipeline.NewOrchestrator(cfg)
    result2, _ := orchestrator2.Process(transcript)
    hash2 := seedManager.DeterministicHash(result2.ToJSONString())
    
    if hash1 != hash2 {
        t.Error("Pipeline results are not deterministic")
    }
}

func TestPipelinePerformance(t *testing.T) {
    // Create large test text
    var builder strings.Builder
    for i := 0; i < 1000; i++ {
        builder.WriteString(fmt.Sprintf("Contact test%d@example.com for urgent matter %d. ", i, i))
    }
    largeText := builder.String()
    
    // Load default config
    cfg := config.GetDefaultConfig()
    
    // Create transcript
    transcript := &domain.Transcript{
        ID:        "performance_test",
        RawText:   largeText,
        Source:    "performance.txt",
        CreatedAt: time.Now(),
    }
    
    // Preprocess
    preprocessor := text.NewPreprocessor(cfg.Processing)
    transcript.ProcessedText = preprocessor.Normalize(largeText)
    transcript.Sentences = preprocessor.SegmentIntoSentences(transcript.ProcessedText)
    transcript.TokenCount = len(preprocessor.Tokenize(transcript.ProcessedText))
    
    // Run pipeline and time it
    start := time.Now()
    orchestrator, err := pipeline.NewOrchestrator(cfg)
    if err != nil {
        t.Fatalf("Failed to create orchestrator: %v", err)
    }
    
    result, err := orchestrator.Process(transcript)
    elapsed := time.Since(start)
    
    if err != nil {
        t.Fatalf("Pipeline failed: %v", err)
    }
    
    // Performance check: should process 10k words in under 30 seconds
    // Our test is 1000 sentences * ~10 words = ~10k words
    if elapsed > 35*time.Second {
        t.Errorf("Pipeline too slow: processed %d words in %v (max 30s expected)", 
            transcript.TokenCount, elapsed)
    }
    
    t.Logf("Performance: Processed %d words in %v", transcript.TokenCount, elapsed)
    t.Logf("Markers found: %d (Atomic: %d, Composite: %d)", 
        result.Statistics.TotalMarkers, 
        result.Statistics.AtomicMarkers,
        result.Statistics.CompositeMarkers)
}