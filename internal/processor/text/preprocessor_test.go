package text_test

import (
    "strings"
    "testing"
    "unicode"

    "semantic-analyzer/internal/processor/text"
    "semantic-analyzer/pkg/config"
)

func TestPreprocessor_Normalize(t *testing.T) {
    cfg := config.GetDefaultProcessingConfig()
    preprocessor := text.NewPreprocessor(cfg)
    
    testCases := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "basic normalization",
            input:    "Hello   World!",
            expected: "hello world!",
        },
        {
            name:     "unicode normalization",
            input:    "café",
            expected: "café",
        },
        {
            name:     "case preservation when disabled",
            input:    "Hello WORLD",
            expected: "hello world",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result := preprocessor.Normalize(tc.input)
            if result != tc.expected {
                t.Errorf("Normalize(%q) = %q, want %q", tc.input, result, tc.expected)
            }
        })
    }
}

func TestPreprocessor_Tokenize(t *testing.T) {
    cfg := config.GetDefaultProcessingConfig()
    preprocessor := text.NewPreprocessor(cfg)
    
    text := "Hello world! This is a test."
    tokens := preprocessor.Tokenize(text)
    
    expected := []string{"hello", "world!", "this", "is", "a", "test."}
    
    if len(tokens) != len(expected) {
        t.Fatalf("Expected %d tokens, got %d", len(expected), len(tokens))
    }
    
    for i, token := range tokens {
        if token != expected[i] {
            t.Errorf("Token %d: got %q, want %q", i, token, expected[i])
        }
    }
}

func TestPreprocessor_SegmentIntoSentences(t *testing.T) {
    cfg := config.GetDefaultProcessingConfig()
    preprocessor := text.NewPreprocessor(cfg)
    
    text := "Hello world! This is a test. How are you?"
    sentences := preprocessor.SegmentIntoSentences(text)
    
    expected := []string{
        "hello world!",
        "this is a test.",
        "how are you?",
    }
    
    if len(sentences) != len(expected) {
        t.Fatalf("Expected %d sentences, got %d", len(expected), len(sentences))
    }
    
    for i, sentence := range sentences {
        if sentence != expected[i] {
            t.Errorf("Sentence %d: got %q, want %q", i, sentence, expected[i])
        }
    }
}

func TestPreprocessor_ValidateEncoding(t *testing.T) {
    cfg := config.GetDefaultProcessingConfig()
    preprocessor := text.NewPreprocessor(cfg)
    
    // Test valid UTF-8
    valid, msg := preprocessor.ValidateEncoding("Hello world")
    if !valid {
        t.Errorf("Valid UTF-8 marked invalid: %s", msg)
    }
    
    // Test invalid UTF-8 (partial sequence)
    invalid := string([]byte{0xff, 0xfe, 0xfd})
    valid, msg = preprocessor.ValidateEncoding(invalid)
    if valid {
        t.Error("Invalid UTF-8 marked valid")
    }
    if msg == "" {
        t.Error("Expected error message for invalid encoding")
    }
}

func TestPreprocessor_RemoveStopWords(t *testing.T) {
    cfg := config.GetDefaultProcessingConfig()
    cfg.StopWords = []string{"the", "a", "an", "and", "or", "but"}
    preprocessor := text.NewPreprocessor(cfg)
    
    tokens := []string{"the", "quick", "brown", "fox", "and", "the", "dog"}
    filtered := preprocessor.RemoveStopWords(tokens)
    
    expected := []string{"quick", "brown", "fox", "dog"}
    
    if len(filtered) != len(expected) {
        t.Fatalf("Expected %d tokens after filtering, got %d", len(expected), len(filtered))
    }
    
    for i, token := range filtered {
        if token != expected[i] {
            t.Errorf("Token %d: got %q, want %q", i, token, expected[i])
        }
    }
}

func TestPreprocessor_GetTextStatistics(t *testing.T) {
    cfg := config.GetDefaultProcessingConfig()
    preprocessor := text.NewPreprocessor(cfg)
    
    text := "Hello world! This is a test. It has multiple sentences."
    stats := preprocessor.GetTextStatistics(text)
    
    if stats["character_count"].(int) != len([]rune(text)) {
        t.Errorf("Character count mismatch")
    }
    
    if stats["sentence_count"].(int) != 3 {
        t.Errorf("Expected 3 sentences, got %d", stats["sentence_count"])
    }
    
    if stats["avg_word_length"].(float64) <= 0 {
        t.Errorf("Average word length should be positive")
    }
}

func BenchmarkPreprocessor_Tokenize(b *testing.B) {
    cfg := config.GetDefaultProcessingConfig()
    preprocessor := text.NewPreprocessor(cfg)
    
    // Create a 1000-word test text
    var builder strings.Builder
    for i := 0; i < 1000; i++ {
        builder.WriteString("word ")
    }
    text := builder.String()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        preprocessor.Tokenize(text)
    }
}

func BenchmarkPreprocessor_SegmentIntoSentences(b *testing.B) {
    cfg := config.GetDefaultProcessingConfig()
    preprocessor := text.NewPreprocessor(cfg)
    
    // Create a test text with 100 sentences
    var builder strings.Builder
    for i := 0; i < 100; i++ {
        builder.WriteString("This is sentence number ")
        builder.WriteString(string(rune('0' + i%10)))
        builder.WriteString(". ")
    }
    text := builder.String()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        preprocessor.SegmentIntoSentences(text)
    }
}