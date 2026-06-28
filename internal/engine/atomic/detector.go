package atomic

import (
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"semantic-analyzer/internal/domain"
	"semantic-analyzer/internal/rules"
	"semantic-analyzer/pkg/determinism"
)

// Detector performs atomic marker detection
type Detector struct {
	ruleEngine   *rules.RuleEngine
	seedManager  *determinism.SeedManager
	textIndex    *TextIndex
	auditEvents  []domain.AuditEvent
	mu           sync.RWMutex
	config       DetectorConfig
	traceID      string // для provenance и трассировки
}

// DetectorConfig contains detector configuration
type DetectorConfig struct {
	MaxWorkers           int
	EnableParallel       bool
	MinConfidence        float64   // будет конвертироваться в fixed-point
	MaxSpanLength        int
	EnableOverlapCheck   bool
	ContextWindowSize    int
	EnableIndexing       bool
	IndexGranularity     int
}

// DefaultDetectorConfig returns default detector configuration
func DefaultDetectorConfig() DetectorConfig {
	return DetectorConfig{
		MaxWorkers:         10,
		EnableParallel:     true,
		MinConfidence:      0.3,
		MaxSpanLength:      1000,
		EnableOverlapCheck: true,
		ContextWindowSize:  50,
		EnableIndexing:     true,
		IndexGranularity:   10,
	}
}

// TextIndex provides fast text search capabilities
type TextIndex struct {
	text           string
	runes          []rune
	charToPosition map[rune][]int
	wordPositions  map[string][]int
	granularity    int
	mu             sync.RWMutex
}

// NewTextIndex creates a new text index
func NewTextIndex(text string, granularity int) *TextIndex {
	idx := &TextIndex{
		text:           text,
		runes:          []rune(text),
		charToPosition: make(map[rune][]int),
		wordPositions:  make(map[string][]int),
		granularity:    granularity,
	}

	idx.buildIndex()
	return idx
}

// buildIndex builds the text index
func (idx *TextIndex) buildIndex() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Index characters at granular intervals
	for i, r := range idx.runes {
		if i%idx.granularity == 0 {
			idx.charToPosition[r] = append(idx.charToPosition[r], i)
		}
	}

	// Index words (simple whitespace tokenization)
	words := strings.Fields(idx.text)
	currentPos := 0
	for _, word := range words {
		pos := strings.Index(idx.text[currentPos:], word)
		if pos != -1 {
			absPos := currentPos + pos
			idx.wordPositions[strings.ToLower(word)] = append(idx.wordPositions[strings.ToLower(word)], absPos)
			currentPos = absPos + len(word)
		}
	}
}

// FindAll returns all positions of a substring (case-insensitive)
func (idx *TextIndex) FindAll(substr string) []int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var positions []int
	lowerText := strings.ToLower(idx.text)
	lowerSubstr := strings.ToLower(substr)

	start := 0
	for {
		pos := strings.Index(lowerText[start:], lowerSubstr)
		if pos == -1 {
			break
		}
		absPos := start + pos
		positions = append(positions, absPos)
		start = absPos + len(substr)
	}

	return positions
}

// FindNear finds positions near a given location
func (idx *TextIndex) FindNear(substr string, center, radius int) []int {
	allPositions := idx.FindAll(substr)
	var nearPositions []int

	for _, pos := range allPositions {
		if domain.Abs(pos-center) <= radius {
			nearPositions = append(nearPositions, pos)
		}
	}

	return nearPositions
}

// GetContext returns text around a position
func (idx *TextIndex) GetContext(position, contextSize int) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	start := domain.Max(0, position-contextSize)
	end := domain.Min(len(idx.runes), position+contextSize)

	return string(idx.runes[start:end])
}

// NewDetector creates a new atomic detector
func NewDetector(ruleEngine *rules.RuleEngine, seedManager *determinism.SeedManager) *Detector {
	return &Detector{
		ruleEngine:  ruleEngine,
		seedManager: seedManager,
		auditEvents: []domain.AuditEvent{},
		config:      DefaultDetectorConfig(),
	}
}

// NewDetectorWithConfig creates a new detector with custom configuration
func NewDetectorWithConfig(ruleEngine *rules.RuleEngine, seedManager *determinism.SeedManager, config DetectorConfig) *Detector {
	return &Detector{
		ruleEngine:  ruleEngine,
		seedManager: seedManager,
		auditEvents: []domain.AuditEvent{},
		config:      config,
	}
}

// Detect performs atomic marker detection on text
func (d *Detector) Detect(text string) ([]domain.Marker, []domain.AuditEvent, error) {
	startTime := time.Now()

	// Set trace ID for provenance and traceability
	d.traceID = d.seedManager.DeterministicHash(text)

	// Create audit event for detection start
	startEvent := domain.NewAuditEvent("atomic_detection", "start")
	startEvent.AddData("text_length", len(text))
	startEvent.AddData("trace_id", d.traceID)
	startEvent.AddData("config", d.config)
	d.addAuditEvent(startEvent)

	var markers []domain.Marker

	// Build text index if enabled
	if d.config.EnableIndexing {
		d.textIndex = NewTextIndex(text, d.config.IndexGranularity)
	}

	// Apply regex rules
	regexMarkers, regexEvents := d.detectWithRegex(text)
	markers = append(markers, regexMarkers...)
	d.auditEvents = append(d.auditEvents, regexEvents...)

	// Apply keyword rules
	keywordMarkers, keywordEvents := d.detectWithKeywords(text)
	markers = append(markers, keywordMarkers...)
	d.auditEvents = append(d.auditEvents, keywordEvents...)

	// Filter markers by confidence (fixed-point)
	filteredMarkers := d.filterMarkers(markers)

	// Apply negative patterns (если есть список — можно передавать через config)
	filteredMarkers = d.ApplyNegativePatterns(filteredMarkers, []string{"не", "отменить"})

	// Remove overlapping markers if enabled
	if d.config.EnableOverlapCheck {
		filteredMarkers = d.removeOverlappingMarkers(filteredMarkers)
	}

	// Sort markers deterministically
	d.sortMarkersDeterministically(filteredMarkers)

	// Add context to markers
	d.addContextToMarkers(filteredMarkers, text)

	// Add provenance, quality, HIL to each marker
	for i := range filteredMarkers {
		filteredMarkers[i].AddMetadata("trace_id", d.traceID)
		filteredMarkers[i].AddMetadata("config_version", "1.0")
		filteredMarkers[i].QualityBucket = "medium"
		filteredMarkers[i].HumanInLoop = false
	}

	// Create audit event for detection end
	endEvent := domain.NewAuditEvent("atomic_detection", "complete")
	endEvent.AddData("markers_found", len(filteredMarkers))
	endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
	endEvent.AddData("trace_id", d.traceID)
	endEvent.SetDuration(startTime)
	d.addAuditEvent(endEvent)

	return filteredMarkers, d.auditEvents, nil
}

// ApplyNegativePatterns filters out markers that match any negative pattern
// and correctly sets the NegativeHit flag on the original marker.
func (d *Detector) ApplyNegativePatterns(markers []domain.Marker, negativePatterns []string) []domain.Marker {
	var filtered []domain.Marker
	for i := range markers {
		m := &markers[i] // работаем с указателем на элемент среза
		hit := false
		for _, pat := range negativePatterns {
			if strings.Contains(strings.ToLower(m.TextSpan), strings.ToLower(pat)) {
				hit = true
				break
			}
		}
		if hit {
			m.NegativeHit = true
		} else {
			filtered = append(filtered, markers[i])
		}
	}
	return filtered
}

// detectWithRegex applies regex rules to text
func (d *Detector) detectWithRegex(text string) ([]domain.Marker, []domain.AuditEvent) {
	startTime := time.Now()
	event := domain.NewAuditEvent("regex_detection", "start")
	event.AddData("text_length", len(text))
	event.AddData("trace_id", d.traceID)
	d.addAuditEvent(event)

	var markers []domain.Marker
	regexRules := d.ruleEngine.GetRegexRules()

	for _, rule := range regexRules {
		ruleStart := time.Now()
		ruleEvent := domain.NewAuditEvent("regex_rule", "apply")
		ruleEvent.RuleID = rule.ID
		ruleEvent.AddData("trace_id", d.traceID)

		matches := rule.Regex.FindAllStringIndex(text, -1)
		ruleEvent.AddData("matches_found", len(matches))

		for _, match := range matches {
			start, end := match[0], match[1]

			// Skip if span too long
			if end-start > d.config.MaxSpanLength {
				continue
			}

			textSpan := text[start:end]

			marker := domain.NewMarker(
				rule.Level,
				rule.Type,
				textSpan,
				start,
				end,
				domain.ToFixedPoint(rule.Weight), // fixed-point
				rule.ID,
				d.traceID,
				"1.0", // config version
				true,
			)

			// Add rule metadata
			marker.AddMetadata("rule_name", rule.Name)
			marker.AddMetadata("detection_method", "regex")

			markers = append(markers, *marker)
		}

		ruleEvent.SetDuration(ruleStart)
		d.addAuditEvent(ruleEvent)
	}

	event = domain.NewAuditEvent("regex_detection", "complete")
	event.AddData("markers_found", len(markers))
	event.AddData("trace_id", d.traceID)
	event.SetDuration(startTime)
	d.addAuditEvent(event)

	return markers, d.auditEvents
}

// detectWithKeywords applies keyword rules to text
// detectWithKeywords applies keyword rules to text
func (d *Detector) detectWithKeywords(text string) ([]domain.Marker, []domain.AuditEvent) {
	startTime := time.Now()
	event := domain.NewAuditEvent("keyword_detection", "start")
	event.AddData("text_length", len(text))
	event.AddData("trace_id", d.traceID)
	d.addAuditEvent(event)

	var markers []domain.Marker
	keywordRules := d.ruleEngine.GetKeywordRules()

	// Prepare text for case-insensitive search if needed
	searchText := text
	lowerText := strings.ToLower(text)

	// Собираем все ключевые слова для быстрого поиска (для expandSpan)
	allKeywords := make(map[string]bool)
	for _, rule := range keywordRules {
		for _, kw := range rule.Keywords {
			allKeywords[strings.ToLower(kw)] = true
		}
	}

	for _, rule := range keywordRules {
		ruleStart := time.Now()
		ruleEvent := domain.NewAuditEvent("keyword_rule", "apply")
		ruleEvent.RuleID = rule.ID
		ruleEvent.AddData("trace_id", d.traceID)

		var matchesFound int

		for _, keyword := range rule.Keywords {
			var positions []int

			if rule.CaseSensitive {
				positions = d.findKeywordPositions(searchText, keyword)
			} else {
				positions = d.findKeywordPositions(lowerText, strings.ToLower(keyword))
			}

			matchesFound += len(positions)

			for _, pos := range positions {
				end := pos + len(keyword)

				// Расширяем захват до полной фразы
				spanStart, spanEnd := d.expandSpan(text, pos, end, allKeywords)
				textSpan := text[spanStart:spanEnd]

				// ----- ОТЛАДОЧНЫЙ ЛОГ -----
				log.Printf("DEBUG keyword marker: type=%s, text=%q, start=%d, end=%d, rule=%s", rule.Type, textSpan, spanStart, spanEnd, rule.ID)

				marker := domain.NewMarker(
					rule.Level,
					rule.Type,
					textSpan,
					spanStart,
					spanEnd,
					domain.ToFixedPoint(rule.Weight),
					rule.ID,
					d.traceID,
					"1.0",
					true,
				)

				// Add rule metadata
				marker.AddMetadata("rule_name", rule.Name)
				marker.AddMetadata("keyword", keyword)
				marker.AddMetadata("detection_method", "keyword")
				marker.AddMetadata("case_sensitive", rule.CaseSensitive)

				markers = append(markers, *marker)
			}
		}

		ruleEvent.AddData("matches_found", matchesFound)
		ruleEvent.SetDuration(ruleStart)
		d.addAuditEvent(ruleEvent)
	}

	event = domain.NewAuditEvent("keyword_detection", "complete")
	event.AddData("markers_found", len(markers))
	event.AddData("trace_id", d.traceID)
	event.SetDuration(startTime)
	d.addAuditEvent(event)

	return markers, d.auditEvents
}

// expandSpan расширяет позицию вокруг ключевого слова до границ предложения или
// до ближайшего другого ключевого слова.
func (d *Detector) expandSpan(text string, start, end int, allKeywords map[string]bool) (int, int) {
	// Расширяем влево до начала предложения
	left := start
	for left > 0 {
		// Остановка на знаках конца предложения или переводе строки
		if text[left-1] == '.' || text[left-1] == '!' || text[left-1] == '?' || text[left-1] == '\n' {
			break
		}
		left--
	}

	// Расширяем вправо до конца предложения
	right := end
	for right < len(text) {
		if text[right] == '.' || text[right] == '!' || text[right] == '?' || text[right] == '\n' {
			right++ // включим знак препинания
			break
		}
		right++
	}

	// Обрезаем пробелы по краям
	for left < right && text[left] == ' ' {
		left++
	}
	for right > left && text[right-1] == ' ' {
		right--
	}
	return left, right
}

// findKeywordPositions finds all positions of a keyword in text
func (d *Detector) findKeywordPositions(text, keyword string) []int {
	if d.textIndex != nil && d.config.EnableIndexing {
		return d.textIndex.FindAll(keyword)
	}

	// Fallback to string search
	var positions []int
	start := 0

	for {
		pos := strings.Index(text[start:], keyword)
		if pos == -1 {
			break
		}

		absPos := start + pos
		positions = append(positions, absPos)
		start = absPos + len(keyword)
	}

	return positions
}

// filterMarkers filters markers by confidence and other criteria
func (d *Detector) filterMarkers(markers []domain.Marker) []domain.Marker {
    var filtered []domain.Marker
    minConf := domain.ToFixedPoint(d.config.MinConfidence)
    for _, marker := range markers {
        if marker.Confidence < minConf { continue }
        if marker.End-marker.Start > d.config.MaxSpanLength { continue }
        // Минимальная длина для действий — 10 символов (чтобы отсеять "нужно")
        if (marker.Type == "ACTION" || marker.Type == "TASK" || marker.Type == "TODO") && len(marker.TextSpan) < 10 {
            continue
        }
        filtered = append(filtered, marker)
    }
    return filtered
}

// removeOverlappingMarkers removes or merges overlapping markers
// removeOverlappingMarkers removes or merges overlapping markers,
// but keeps markers of different types even if they overlap.
func (d *Detector) removeOverlappingMarkers(markers []domain.Marker) []domain.Marker {
	if len(markers) == 0 {
		return markers
	}

	// Sort by start position
	sort.Slice(markers, func(i, j int) bool {
		if markers[i].Start == markers[j].Start {
			return markers[i].End < markers[j].End
		}
		return markers[i].Start < markers[j].Start
	})

	var result []domain.Marker
	current := markers[0]

	for i := 1; i < len(markers); i++ {
		next := markers[i]

		if current.Overlaps(&next) && current.Type == next.Type {
			// Same type: keep the one with higher confidence
			if current.Confidence >= next.Confidence {
				continue
			} else {
				current = next
			}
		} else {
			// Different types or no overlap: keep both
			result = append(result, current)
			current = next
		}
	}

	result = append(result, current)
	return result
}

// sortMarkersDeterministically sorts markers deterministically
func (d *Detector) sortMarkersDeterministically(markers []domain.Marker) {
	// Используем stable sort из seedManager
	d.seedManager.StableSort(markers)
}

// addContextToMarkers adds context to markers
func (d *Detector) addContextToMarkers(markers []domain.Marker, text string) {
	for i := range markers {
		markers[i].SetContext(text, d.config.ContextWindowSize)
	}
}

// addAuditEvent adds an audit event
func (d *Detector) addAuditEvent(event *domain.AuditEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.auditEvents = append(d.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (d *Detector) GetAuditEvents() []domain.AuditEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.auditEvents
}

// ClearAuditEvents clears all audit events
func (d *Detector) ClearAuditEvents() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.auditEvents = []domain.AuditEvent{}
}

// SetConfig updates detector configuration
func (d *Detector) SetConfig(config DetectorConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.config = config
}

// GetConfig returns current detector configuration
func (d *Detector) GetConfig() DetectorConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config
}

// Benchmark performs a benchmark test
func (d *Detector) Benchmark(text string, iterations int) map[string]interface{} {
	results := make(map[string]interface{})

	// Time detection
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, _, err := d.Detect(text)
		if err != nil {
			results["error"] = err.Error()
			return results
		}
		d.ClearAuditEvents()
	}
	totalTime := time.Since(start)

	// Run single detection for metrics
	markers, _, _ := d.Detect(text)

	results["iterations"] = iterations
	results["total_time"] = totalTime.String()
	results["avg_time_per_iteration"] = (totalTime / time.Duration(iterations)).String()
	results["markers_found"] = len(markers)
	results["text_length"] = len(text)
	results["throughput_chars_per_sec"] = float64(len(text)*iterations) / totalTime.Seconds()

	return results
}