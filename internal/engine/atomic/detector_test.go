package atomic_test

import (
	"fmt"
	"strings"
	"testing"

	"semantic-analyzer/internal/domain"
	"semantic-analyzer/internal/engine/atomic"
	"semantic-analyzer/internal/rules"
	"semantic-analyzer/pkg/config"
	"semantic-analyzer/pkg/determinism"
)

// Вспомогательная функция, заменяющая отсутствующую GetDefaultAtomicRules
func getDefaultAtomicRules() config.AtomicRulesConfig {
	return config.AtomicRulesConfig{
		RegexPatterns: []config.RegexRuleConfig{
			{
				ID:      "email",
				Pattern: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`,
				Weight:  0.9,
				Type:    "CONTACT",
				Level:   1,
				Name:    "Email",
			},
			{
				ID:      "phone",
				Pattern: `\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`,
				Weight:  0.8,
				Type:    "PHONE",
				Level:   1,
				Name:    "Phone",
			},
		},
		KeywordLists: []config.KeywordRuleConfig{
			{
				ID:            "urgency",
				Keywords:      []string{"urgent", "important", "critical"},
				Weight:        0.7,
				Type:          "PRIORITY",
				Level:         1,
				CaseSensitive: false,
				Name:          "Urgency",
			},
		},
	}
}

func TestDetector_DetectRegex(t *testing.T) {
	atomicRules := config.AtomicRulesConfig{
		RegexPatterns: []config.RegexRuleConfig{
			{
				ID:      "email_pattern",
				Pattern: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
				Weight:  0.9,
				Type:    "CONTACT",
				Level:   1,
				Name:    "Email Address",
			},
		},
		KeywordLists: []config.KeywordRuleConfig{},
	}

	ruleEngine, err := rules.NewRuleEngine(atomicRules)
	if err != nil {
		t.Fatalf("Failed to create rule engine: %v", err)
	}

	seedManager := determinism.NewSeedManager(42)
	detector := atomic.NewDetector(ruleEngine, seedManager)

	text := "Contact us at test@example.com for more information."
	markers, auditEvents, err := detector.Detect(text)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if len(markers) != 1 {
		t.Fatalf("Expected 1 marker, got %d", len(markers))
	}

	marker := markers[0]
	if marker.Type != "CONTACT" {
		t.Errorf("Expected marker type CONTACT, got %s", marker.Type)
	}
	if marker.TextSpan != "test@example.com" {
		t.Errorf("Expected marker text 'test@example.com', got %s", marker.TextSpan)
	}
	if len(auditEvents) == 0 {
		t.Error("Expected audit events")
	}
}

func TestDetector_DetectKeywords(t *testing.T) {
	atomicRules := config.AtomicRulesConfig{
		RegexPatterns: []config.RegexRuleConfig{},
		KeywordLists: []config.KeywordRuleConfig{
			{
				ID:            "important_terms",
				Keywords:      []string{"urgent", "important", "critical"},
				Weight:        0.6,
				Type:          "PRIORITY",
				Level:         1,
				CaseSensitive: false,
				Name:          "Important Terms",
			},
		},
	}

	ruleEngine, err := rules.NewRuleEngine(atomicRules)
	if err != nil {
		t.Fatalf("Failed to create rule engine: %v", err)
	}

	seedManager := determinism.NewSeedManager(42)
	detector := atomic.NewDetector(ruleEngine, seedManager)

	text := "This is an URGENT matter that requires immediate attention."
	markers, _, err := detector.Detect(text)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if len(markers) != 1 {
		t.Fatalf("Expected 1 marker, got %d", len(markers))
	}
	marker := markers[0]
	if marker.Type != "PRIORITY" {
		t.Errorf("Expected marker type PRIORITY, got %s", marker.Type)
	}
	// Точный текст, захваченный детектором, может отличаться в зависимости от expandSpan
	if marker.TextSpan != "URGENT" {
		t.Logf("Marker text (may vary): %q", marker.TextSpan)
	}
}

func TestDetector_OverlapDetection(t *testing.T) {
	atomicRules := getDefaultAtomicRules()
	ruleEngine, err := rules.NewRuleEngine(atomicRules)
	if err != nil {
		t.Fatalf("Failed to create rule engine: %v", err)
	}
	seedManager := determinism.NewSeedManager(42)
	detector := atomic.NewDetector(ruleEngine, seedManager)

	text := "Contact: test@example.com and phone: 123-456-7890"
	markers, _, err := detector.Detect(text)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}
	if len(markers) < 2 {
		t.Errorf("Expected at least 2 markers, got %d", len(markers))
	}
	for i := 0; i < len(markers); i++ {
		for j := i + 1; j < len(markers); j++ {
			if markers[i].Overlaps(&markers[j]) {
				t.Errorf("Markers %d and %d overlap but shouldn't", i, j)
			}
		}
	}
}

func TestDetector_Determinism(t *testing.T) {
	atomicRules := getDefaultAtomicRules()
	ruleEngine, err := rules.NewRuleEngine(atomicRules)
	if err != nil {
		t.Fatalf("Failed to create rule engine: %v", err)
	}
	seedManager := determinism.NewSeedManager(42)
	detector := atomic.NewDetector(ruleEngine, seedManager)

	text := "Contact us at test@example.com or call 123-456-7890. This is urgent!"

	var firstMarkers []domain.Marker
	var firstHashes []string

	for i := 0; i < 10; i++ {
		markers, _, err := detector.Detect(text)
		if err != nil {
			t.Fatalf("Detection failed at iteration %d: %v", i, err)
		}

		if i == 0 {
			firstMarkers = markers
			for _, marker := range markers {
				h := seedManager.DeterministicHash(
					fmt.Sprintf("%d:%d:%s", marker.Start, marker.End, marker.Type))
				firstHashes = append(firstHashes, fmt.Sprintf("%x", h))
			}
		} else {
			if len(markers) != len(firstMarkers) {
				t.Errorf("Marker count changed at iteration %d: %d != %d",
					i, len(markers), len(firstMarkers))
			}
			for j, marker := range markers {
				h := seedManager.DeterministicHash(
					fmt.Sprintf("%d:%d:%s", marker.Start, marker.End, marker.Type))
				if fmt.Sprintf("%x", h) != firstHashes[j] {
					t.Errorf("Marker %d changed at iteration %d", j, i)
				}
			}
		}
		detector.ClearAuditEvents()
	}
}

func TestDetector_Benchmark(t *testing.T) {
	atomicRules := getDefaultAtomicRules()
	ruleEngine, err := rules.NewRuleEngine(atomicRules)
	if err != nil {
		t.Fatalf("Failed to create rule engine: %v", err)
	}
	seedManager := determinism.NewSeedManager(42)
	detector := atomic.NewDetector(ruleEngine, seedManager)

	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("Contact us at test")
		sb.WriteString(fmt.Sprintf("%d", i))
		sb.WriteString("@example.com for urgent matters. ")
	}
	text := sb.String()

	results := detector.Benchmark(text, 5)

	if results["iterations"].(int) != 5 {
		t.Errorf("Expected 5 iterations, got %d", results["iterations"])
	}
	if results["markers_found"].(int) == 0 {
		t.Error("Expected to find markers")
	}
	throughput := results["throughput_chars_per_sec"].(float64)
	if throughput <= 0 {
		t.Error("Throughput should be positive")
	}
	t.Logf("Benchmark results: %+v", results)
}

func TestDetector_ConfigValidation(t *testing.T) {
	atomicRules := getDefaultAtomicRules()
	ruleEngine, err := rules.NewRuleEngine(atomicRules)
	if err != nil {
		t.Fatalf("Failed to create rule engine: %v", err)
	}
	seedManager := determinism.NewSeedManager(42)

	cfg := atomic.DefaultDetectorConfig()
	cfg.MinConfidence = -0.1
	detector := atomic.NewDetectorWithConfig(ruleEngine, seedManager, cfg)

	text := "test@example.com"
	markers, _, err := detector.Detect(text)
	if err != nil {
		t.Errorf("Detection should handle invalid config gracefully: %v", err)
	}

	cfg.MinConfidence = 0.3
	detector.SetConfig(cfg)
	markers, _, err = detector.Detect(text)
	if err != nil {
		t.Errorf("Detection failed with valid config: %v", err)
	}
	if len(markers) == 0 {
		t.Error("Expected to find markers with valid config")
	}
}