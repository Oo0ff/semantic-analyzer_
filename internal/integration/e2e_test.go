package integration_test

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"

    "semantic-analyzer/cmd/pipeline"
    "semantic-analyzer/internal/domain"
    "semantic-analyzer/internal/processor/text"
    "semantic-analyzer/pkg/config"
    "semantic-analyzer/pkg/determinism"
)

// TestDeterminism runs the same analysis multiple times to ensure determinism
func TestDeterminism(t *testing.T) {
    // Load test configuration
    cfg := config.GetDefaultConfig()
    cfg.Determinism.BaseSeed = 12345
    
    // Create test transcript
    testText := `Meeting Notes - Project Review
    
    Date: 2024-01-15
    Participants: john@example.com, jane@company.com
    
    URGENT: Need to finalize requirements by Friday.
    
    Action Items:
    1. Update timeline - Due: 2024-01-20
    2. Contact vendor - Email: vendor@example.com
    3. Prepare presentation
    
    Important: This is a high priority project.`
    
    transcript := &domain.Transcript{
        ID:        "test_determinism",
        RawText:   testText,
        Source:    "test.txt",
        CreatedAt: time.Now(),
    }
    
    // Preprocess
    preprocessor := text.NewPreprocessor(cfg.Processing)
    transcript.ProcessedText = preprocessor.Normalize(testText)
    transcript.Sentences = preprocessor.SegmentIntoSentences(transcript.ProcessedText)
    transcript.TokenCount = len(preprocessor.Tokenize(transcript.ProcessedText))
    
    var firstHash string
    var firstResult *domain.AnalysisResult
    
    // Run analysis 10 times
    for i := 0; i < 10; i++ {
        orchestrator, err := pipeline.NewOrchestrator(cfg)
        if err != nil {
            t.Fatalf("Failed to create orchestrator: %v", err)
        }
        
        result, err := orchestrator.Process(transcript)
        if err != nil {
            t.Fatalf("Analysis failed: %v", err)
        }
        
        // Calculate hash of result
        jsonData, err := json.Marshal(result)
        if err != nil {
            t.Fatalf("Failed to marshal result: %v", err)
        }
        
        hash := sha256.Sum256(jsonData)
        hashStr := hex.EncodeToString(hash[:])
        
        if i == 0 {
            firstHash = hashStr
            firstResult = result
        } else {
            if hashStr != firstHash {
                t.Errorf("Run %d: Hash mismatch (non-deterministic behavior)", i)
                t.Errorf("Expected: %s", firstHash)
                t.Errorf("Got:      %s", hashStr)
                
                // Debug: compare marker counts
                if firstResult != nil {
                    t.Errorf("First run markers: %d", firstResult.Statistics.TotalMarkers)
                    t.Errorf("This run markers: %d", result.Statistics.TotalMarkers)
                }
            }
        }
    }
    
    t.Logf("Determinism test passed: 10 runs produced identical results (hash: %s)", firstHash[:16])
}

// TestPerformance runs performance test with 10k words
func TestPerformance(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping performance test in short mode")
    }
    
    cfg := config.GetDefaultConfig()
    
    // Generate 10k word test text
    testText := generateTestText(10000)
    
    transcript := &domain.Transcript{
        ID:        "test_performance",
        RawText:   testText,
        Source:    "performance_test.txt",
        CreatedAt: time.Now(),
    }
    
    // Preprocess
    preprocessor := text.NewPreprocessor(cfg.Processing)
    transcript.ProcessedText = preprocessor.Normalize(testText)
    transcript.Sentences = preprocessor.SegmentIntoSentences(transcript.ProcessedText)
    transcript.TokenCount = len(preprocessor.Tokenize(transcript.ProcessedText))
    
    t.Logf("Test text: %d words, %d sentences", transcript.TokenCount, len(transcript.Sentences))
    
    // Run analysis
    start := time.Now()
    
    orchestrator, err := pipeline.NewOrchestrator(cfg)
    if err != nil {
        t.Fatalf("Failed to create orchestrator: %v", err)
    }
    
    result, err := orchestrator.Process(transcript)
    elapsed := time.Since(start)
    
    if err != nil {
        t.Fatalf("Analysis failed: %v", err)
    }
    
    // Check performance requirement (30 seconds)
    if elapsed > 30*time.Second {
        t.Errorf("Performance test failed: %v > 30s limit", elapsed)
    } else {
        t.Logf("Performance test passed: %v (< 30s)", elapsed)
    }
    
    // Log statistics
    t.Logf("Processing time: %v", elapsed)
    t.Logf("Markers found: %d", result.Statistics.TotalMarkers)
    t.Logf("Words per second: %.0f", float64(transcript.TokenCount)/elapsed.Seconds())
    t.Logf("Memory estimate: %.2f MB", float64(len(testText))/1024/1024)
}

// TestEndToEnd runs complete end-to-end test
func TestEndToEnd(t *testing.T) {
    // Test with reference transcripts
    testCases := []struct {
        name     string
        file     string
        expected struct {
            minMarkers int
            maxMarkers int
        }
    }{
        {
            name: "meeting_notes",
            file: "testdata/meeting_notes.txt",
            expected: struct {
                minMarkers int
                maxMarkers int
            }{5, 20},
        },
        {
            name: "email_thread",
            file: "testdata/email_thread.txt",
            expected: struct {
                minMarkers int
                maxMarkers int
            }{10, 30},
        },
        {
            name: "technical_doc",
            file: "testdata/technical_doc.txt",
            expected: struct {
                minMarkers int
                maxMarkers int
            }{15, 40},
        },
    }
    
    cfg := config.GetDefaultConfig()
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Read test file
            content, err := os.ReadFile(tc.file)
            if err != nil {
                t.Skipf("Test file not found: %s", tc.file)
                return
            }
            
            transcript := &domain.Transcript{
                ID:        tc.name,
                RawText:   string(content),
                Source:    filepath.Base(tc.file),
                CreatedAt: time.Now(),
            }
            
            // Preprocess
            preprocessor := text.NewPreprocessor(cfg.Processing)
            transcript.ProcessedText = preprocessor.Normalize(string(content))
            transcript.Sentences = preprocessor.SegmentIntoSentences(transcript.ProcessedText)
            transcript.TokenCount = len(preprocessor.Tokenize(transcript.ProcessedText))
            
            // Run analysis
            orchestrator, err := pipeline.NewOrchestrator(cfg)
            if err != nil {
                t.Fatalf("Failed to create orchestrator: %v", err)
            }
            
            result, err := orchestrator.Process(transcript)
            if err != nil {
                t.Fatalf("Analysis failed: %v", err)
            }
            
            // Validate results
            markerCount := result.Statistics.TotalMarkers
            
            if markerCount < tc.expected.minMarkers {
                t.Errorf("Too few markers: %d (expected at least %d)", 
                    markerCount, tc.expected.minMarkers)
            }
            
            if markerCount > tc.expected.maxMarkers {
                t.Errorf("Too many markers: %d (expected at most %d)", 
                    markerCount, tc.expected.maxMarkers)
            }
            
            // Check marker levels distribution
            levelCounts := make(map[int]int)
            for _, marker := range result.Markers {
                levelCounts[marker.Level]++
            }
            
            t.Logf("Test %s: %d markers, levels: %v", 
                tc.name, markerCount, levelCounts)
            
            // Check confidence scores are valid
            for _, marker := range result.Markers {
                if marker.Confidence < 0 || marker.Confidence > 1 {
                    t.Errorf("Invalid confidence: %f for marker %s", 
                        marker.Confidence, marker.ID)
                }
            }
            
            // Check audit trail exists
            if result.AuditTrail == nil {
                t.Error("Audit trail missing")
            } else if len(result.AuditTrail.Events) == 0 {
                t.Error("Audit trail empty")
            }
        })
    }
}

// TestErrorHandling tests error handling
func TestErrorHandling(t *testing.T) {
    cfg := config.GetDefaultConfig()
    
    // Test with empty text
    transcript := &domain.Transcript{
        ID:        "test_empty",
        RawText:   "",
        Source:    "empty.txt",
        CreatedAt: time.Now(),
    }
    
    orchestrator, err := pipeline.NewOrchestrator(cfg)
    if err != nil {
        t.Fatalf("Failed to create orchestrator: %v", err)
    }
    
    result, err := orchestrator.Process(transcript)
    if err != nil {
        t.Logf("Empty text handled correctly: %v", err)
        return
    }
    
    // If no error, should have 0 markers
    if result.Statistics.TotalMarkers != 0 {
        t.Errorf("Empty text should produce 0 markers, got %d", 
            result.Statistics.TotalMarkers)
    }
    
    // Test with invalid UTF-8
    transcript.RawText = string([]byte{0xff, 0xfe, 0xfd})
    transcript.ID = "test_invalid_utf8"
    
    result, err = orchestrator.Process(transcript)
    if err == nil {
        t.Error("Expected error for invalid UTF-8")
    } else {
        t.Logf("Invalid UTF-8 handled correctly: %v", err)
    }
}

// Helper function to generate test text
func generateTestText(wordCount int) string {
    baseWords := []string{
        "project", "meeting", "urgent", "email", "phone", "date", "time",
        "action", "task", "priority", "important", "critical", "deadline",
        "timeline", "milestone", "deliverable", "requirement", "specification",
        "contact", "call", "message", "update", "review