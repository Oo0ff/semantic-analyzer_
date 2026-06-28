package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"semantic-analyzer/internal/calendar"
	"semantic-analyzer/internal/classification"
	"semantic-analyzer/internal/domain"
	"semantic-analyzer/internal/engine/atomic"
	"semantic-analyzer/internal/engine/composite"
	"semantic-analyzer/internal/engine/final"
	"semantic-analyzer/internal/engine/semantic"
	"semantic-analyzer/internal/notifications"
	"semantic-analyzer/internal/participants"
	"semantic-analyzer/internal/processor/text"
	"semantic-analyzer/internal/protocol"
	"semantic-analyzer/internal/rules"
	"semantic-analyzer/internal/search"
	"semantic-analyzer/internal/summarization"
	"semantic-analyzer/pkg/config"
	"semantic-analyzer/pkg/determinism"
)

// PhaseResult — результат выполнения одной фазы
type PhaseResult struct {
	PhaseName   string
	Success     bool
	Duration    time.Duration
	AuditEvents []domain.AuditEvent
	Data        map[string]interface{}
	Error       error
}

// PipelineData — данные, передаваемые между фазами
type PipelineData struct {
	Transcript   *domain.Transcript
	StartTime    time.Time
	AuditEvents  []domain.AuditEvent
	PhaseResults map[string]*PhaseResult
	CustomData   map[string]interface{}
}

// Pipeline — оркестратор всего пайплайна
type Pipeline struct {
	phases      []Phase
	config      *config.Config
	seedManager *determinism.SeedManager
	bufferSize  int
	workerPool  chan struct{}
}

// Phase — интерфейс для каждой фазы пайплайна
type Phase interface {
	Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error)
	Name() string
	Description() string
}

// NewPipeline создаёт новый пайплайн
func NewPipeline(cfg *config.Config) *Pipeline {
	seedManager := determinism.NewSeedManager(cfg.Determinism.BaseSeed)

	return &Pipeline{
		config:      cfg,
		seedManager: seedManager,
		bufferSize:  cfg.Performance.BufferSize,
		workerPool:  make(chan struct{}, cfg.Performance.MaxWorkers),
	}
}

// Initialize заполняет список фаз (в правильном порядке)
func (p *Pipeline) Initialize() error {
	p.phases = []Phase{
		NewTextProcessingPhase(p.config, p.seedManager),
		NewAtomicDetectionPhase(p.config, p.seedManager),
		NewCompositeAnalysisPhase(p.config, p.seedManager),
		NewSemanticAnalysisPhase(p.config, p.seedManager),
		NewClassificationPhase(p.config),
		NewSummarizationPhase(p.config),
		NewEventExtractionPhase(p.config),
		NewProtocolGenerationPhase(p.config),
		NewNotificationPhase(p.config),
		NewPostProcessingPhase(p.config, p.seedManager),
		NewIndexingPhase(p.config),
	}
	return nil
}

// Process запускает весь пайплайн
func (p *Pipeline) Process(ctx context.Context, transcript *domain.Transcript) (*domain.AnalysisResult, error) {
	startTime := time.Now()

	data := &PipelineData{
		Transcript:   transcript,
		StartTime:    startTime,
		AuditEvents:  []domain.AuditEvent{},
		PhaseResults: make(map[string]*PhaseResult),
		CustomData:   make(map[string]interface{}),
	}

	for _, phase := range p.phases {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Захватываем слот (ограничивает количество одновременных пайплайнов)
			p.workerPool <- struct{}{}
			result, err := phase.Execute(ctx, data)
			<-p.workerPool // освобождаем слот сразу после выполнения
			if err != nil {
				return nil, fmt.Errorf("phase %s failed: %w", phase.Name(), err)
			}

			data.PhaseResults[phase.Name()] = result
			data.AuditEvents = append(data.AuditEvents, result.AuditEvents...)
		}
	}

	return p.assembleResult(data), nil
}

// assembleResult собирает итоговый AnalysisResult
func (p *Pipeline) assembleResult(data *PipelineData) *domain.AnalysisResult {
	var atomicMarkers, compositeMarkers, semanticMarkers []domain.Marker

	if atomicRes, ok := data.PhaseResults["atomic_detection"]; ok && atomicRes != nil {
		if m, ok := atomicRes.Data["markers"].([]domain.Marker); ok {
			atomicMarkers = m
		}
	}

	if compRes, ok := data.PhaseResults["composite_analysis"]; ok && compRes != nil {
		if m, ok := compRes.Data["composite_markers"].([]domain.Marker); ok {
			compositeMarkers = m
		}
	}

	if semRes, ok := data.PhaseResults["semantic_analysis"]; ok && semRes != nil {
		if m, ok := semRes.Data["semantic_markers"].([]domain.Marker); ok {
			semanticMarkers = m
		}
	}

	allMarkers := append(atomicMarkers, compositeMarkers...)
	allMarkers = append(allMarkers, semanticMarkers...)

	result := &domain.AnalysisResult{
		TranscriptID:   data.Transcript.ID,
		Transcript:     data.Transcript,
		Markers:        allMarkers,
		AuditEvents:    data.AuditEvents,
		Statistics: domain.Statistics{
			TotalMarkers:      len(allMarkers),
			AtomicMarkers:     len(atomicMarkers),
			CompositeMarkers:  len(compositeMarkers) + len(semanticMarkers),
			ProcessingTime:    time.Since(data.StartTime).Seconds(),
			AverageConfidence: calculateAverageConfidence(allMarkers),
			MarkerTypes:       countMarkerTypes(allMarkers),
			SentenceCount:     len(data.Transcript.Sentences),
			WordCount:         data.Transcript.TokenCount,
		},
		Timestamp:      time.Now(),
		ConfigHash:     p.seedManager.DeterministicHash(fmt.Sprintf("%v", p.config)),
		ProcessingTime: time.Since(data.StartTime).Seconds(),
		Version:        p.config.App.Version,
		Metadata: map[string]interface{}{
			"pipeline_version": "1.0",
			"phases_executed":  len(p.phases),
			"seed":             p.config.Determinism.BaseSeed,
		},
	}

	for phaseName, res := range data.PhaseResults {
		if res != nil {
			result.Metadata[phaseName+"_duration"] = res.Duration.Seconds()
			if stats, ok := res.Data["statistics"]; ok {
				result.Metadata[phaseName+"_stats"] = stats
			}
		}
	}

	// Тематическая классификация
	if cr, ok := data.PhaseResults["classification"]; ok && cr != nil {
		if topics, ok := cr.Data["topics"].([]domain.TopicResult); ok {
			result.Topics = topics
		}
	}
	// Суммаризация
	if sr, ok := data.PhaseResults["summarization"]; ok && sr != nil {
		if summary, ok := sr.Data["summary"].(string); ok {
			result.Summary = summary
		}
	}

	return result
}

// --- Вспомогательные функции ---

func calculateAverageConfidence(markers []domain.Marker) float64 {
	if len(markers) == 0 {
		return 0.0
	}
	var total float64
	for _, m := range markers {
		total += domain.FromFixedPoint(m.Confidence)
	}
	return total / float64(len(markers))
}

func countMarkerTypes(markers []domain.Marker) map[string]int {
	counts := make(map[string]int)
	for _, m := range markers {
		counts[m.Type]++
	}
	return counts
}

// --- Фазы пайплайна (существующие) ---

// TextProcessingPhase — фаза предобработки текста
type TextProcessingPhase struct {
	config      *config.Config
	seedManager *determinism.SeedManager
}

func NewTextProcessingPhase(cfg *config.Config, sm *determinism.SeedManager) *TextProcessingPhase {
	return &TextProcessingPhase{config: cfg, seedManager: sm}
}

func (p *TextProcessingPhase) Name() string       { return "text_processing" }
func (p *TextProcessingPhase) Description() string { return "Text normalization and preprocessing" }

func (p *TextProcessingPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	locale := p.config.Processing.Locale
	if locale == "" {
		locale = "en" // fallback
	}
	preprocessor := text.NewPreprocessor(p.config.Processing, locale)

	normalized := preprocessor.Normalize(data.Transcript.RawText)
	data.Transcript.ProcessedText = normalized

	sentences := preprocessor.SegmentIntoSentences(normalized)
	data.Transcript.Sentences = sentences

	tokens := preprocessor.Tokenize(normalized)
	data.Transcript.TokenCount = len(tokens)

	stats := preprocessor.GetTextStatistics(normalized)

	completeEvent := domain.NewAuditEvent("text_processing", "complete")
	completeEvent.AddData("normalized_length", len(normalized))

	return &PhaseResult{
		PhaseName:   p.Name(),
		Success:     true,
		Duration:    time.Since(start),
		AuditEvents: []domain.AuditEvent{*completeEvent},
		Data: map[string]interface{}{
			"normalized_text": normalized,
			"sentences":       sentences,
			"statistics":      stats,
		},
	}, nil
}

// AtomicDetectionPhase — фаза обнаружения атомарных маркеров
type AtomicDetectionPhase struct {
	config      *config.Config
	seedManager *determinism.SeedManager
}

func NewAtomicDetectionPhase(cfg *config.Config, sm *determinism.SeedManager) *AtomicDetectionPhase {
	return &AtomicDetectionPhase{config: cfg, seedManager: sm}
}

func (p *AtomicDetectionPhase) Name() string       { return "atomic_detection" }
func (p *AtomicDetectionPhase) Description() string { return "Atomic marker detection" }

func (p *AtomicDetectionPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	ruleEngine, err := rules.NewRuleEngine(p.config.Rules.Atomic)
	if err != nil {
		return nil, fmt.Errorf("failed to init rule engine: %w", err)
	}

	// Загружаем пользовательские словари из CSV (если директория существует)
	if err := ruleEngine.LoadCustomDictionaries("./custom_rules"); err != nil {
		log.Printf("WARNING: failed to load custom dictionaries: %v", err)
	}
	personNames := ruleEngine.GetPersonNames()
	data.Transcript.AddMetadata("custom_persons", personNames)

	detector := atomic.NewDetector(ruleEngine, p.seedManager)

	markers, auditEvents, err := detector.Detect(data.Transcript.ProcessedText)
	if err != nil {
		return nil, fmt.Errorf("atomic detection failed: %w", err)
	}

	return &PhaseResult{
		PhaseName:   p.Name(),
		Success:     true,
		Duration:    time.Since(start),
		AuditEvents: auditEvents,
		Data: map[string]interface{}{
			"markers": markers,
			"statistics": map[string]interface{}{
				"marker_count": len(markers),
				"types_found":  countMarkerTypes(markers),
			},
		},
	}, nil
}

// CompositeAnalysisPhase — фаза композитных маркеров
type CompositeAnalysisPhase struct {
	config      *config.Config
	seedManager *determinism.SeedManager
}

func NewCompositeAnalysisPhase(cfg *config.Config, sm *determinism.SeedManager) *CompositeAnalysisPhase {
	return &CompositeAnalysisPhase{config: cfg, seedManager: sm}
}

func (p *CompositeAnalysisPhase) Name() string       { return "composite_analysis" }
func (p *CompositeAnalysisPhase) Description() string { return "Composite marker analysis" }

func (p *CompositeAnalysisPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	atomicRes, ok := data.PhaseResults["atomic_detection"]
	if !ok || atomicRes == nil {
		return nil, fmt.Errorf("atomic detection required before composite")
	}

	atomicMarkers, ok := atomicRes.Data["markers"].([]domain.Marker)
	if !ok {
		atomicMarkers = []domain.Marker{}
	}

	textRes, ok := data.PhaseResults["text_processing"]
	if !ok || textRes == nil {
		return nil, fmt.Errorf("text processing required before composite")
	}

	sentences, ok := textRes.Data["sentences"].([]string)
	if !ok {
		sentences = []string{}
	}

	seedStr := fmt.Sprintf("%d", p.config.Determinism.BaseSeed)
	builder := composite.NewBuilder(p.config.Rules.Composite, seedStr)
	candidates := builder.BuildContextWindows(atomicMarkers, sentences)

	scorer := composite.NewScorer(p.config.Rules.Composite)
	scored := scorer.ScoreAllCandidates(candidates)

	selector := composite.NewSelector(p.seedManager)
	selected := selector.SelectTopCandidates(scored, p.config.Rules.Composite.MaxCandidates)
	selected = selector.RemoveRedundantCandidates(selected, 0.5)

	compositeMarkers := convertCandidatesToMarkers(selected)

	var auditEvents []domain.AuditEvent
	auditEvents = append(auditEvents, builder.GetAuditEvents()...)
	auditEvents = append(auditEvents, scorer.GetAuditEvents()...)
	auditEvents = append(auditEvents, selector.GetAuditEvents()...)

	return &PhaseResult{
		PhaseName:   p.Name(),
		Success:     true,
		Duration:    time.Since(start),
		AuditEvents: auditEvents,
		Data: map[string]interface{}{
			"composite_candidates": selected,
			"composite_markers":    compositeMarkers,
			"statistics": map[string]interface{}{
				"candidates_created":  len(candidates),
				"candidates_selected": len(selected),
				"composite_markers":   len(compositeMarkers),
			},
		},
	}, nil
}

// SemanticAnalysisPhase — фаза семантического анализа
type SemanticAnalysisPhase struct {
	config      *config.Config
	seedManager *determinism.SeedManager
}

func NewSemanticAnalysisPhase(cfg *config.Config, sm *determinism.SeedManager) *SemanticAnalysisPhase {
	return &SemanticAnalysisPhase{config: cfg, seedManager: sm}
}

func (p *SemanticAnalysisPhase) Name() string       { return "semantic_analysis" }
func (p *SemanticAnalysisPhase) Description() string { return "Semantic analysis and NER" }

func (p *SemanticAnalysisPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	textRes, _ := data.PhaseResults["text_processing"]
	atomicRes, _ := data.PhaseResults["atomic_detection"]

	text := ""
	if r, ok := textRes.Data["normalized_text"].(string); ok {
		text = r
	}

	atomicMarkers := []domain.Marker{}
	if r, ok := atomicRes.Data["markers"].([]domain.Marker); ok {
		atomicMarkers = r
	}

	seedStr := fmt.Sprintf("%d", p.config.Determinism.BaseSeed)
	engine, err := semantic.NewSemanticEngine(p.config.Rules.Semantic, seedStr)
	if err != nil {
		return nil, fmt.Errorf("failed to init semantic engine: %w", err)
	}

	semResult, err := engine.Analyze(text, atomicMarkers, nil)
	if err != nil {
		return nil, fmt.Errorf("semantic analysis failed: %w", err)
	}

	return &PhaseResult{
		PhaseName:   p.Name(),
		Success:     true,
		Duration:    time.Since(start),
		AuditEvents: engine.GetAuditEvents(),
		Data: map[string]interface{}{
			"entities":         semResult.Entities,
			"semantic_markers": semResult.GeneratedMarkers,
			"statistics": map[string]interface{}{
				"entities_found":    len(semResult.Entities),
				"markers_generated": len(semResult.GeneratedMarkers),
			},
		},
	}, nil
}

// ClassificationPhase – фаза тематической классификации
type ClassificationPhase struct {
	config *config.Config
}

func NewClassificationPhase(cfg *config.Config) *ClassificationPhase {
	return &ClassificationPhase{config: cfg}
}

func (p *ClassificationPhase) Name() string       { return "classification" }
func (p *ClassificationPhase) Description() string { return "Topic classification" }

func (p *ClassificationPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	// Преобразуем типы конфигурации
	topics := convertTopics(p.config.Classification.Topics)
	log.Printf("DEBUG: classification topics count = %d", len(topics))
	for _, t := range topics {
		log.Printf("DEBUG: topic %s keywords=%v", t.Code, t.Keywords)
	}

	classifier := classification.New(classification.Config{
		Topics: topics,
	})

	text := data.Transcript.ProcessedText
	log.Printf("DEBUG: ProcessedText length=%d, first 150 chars: %s", len(text), safePrefix(text, 150))

	var markers []domain.Marker
	if ar, ok := data.PhaseResults["atomic_detection"]; ok && ar != nil {
		if m, ok := ar.Data["markers"].([]domain.Marker); ok {
			markers = m
		}
	}
	results := classifier.Classify(text, markers)
	log.Printf("DEBUG: classification results count = %d", len(results))
	for _, r := range results {
		log.Printf("DEBUG: topic %s confidence=%.2f", r.Code, r.Confidence)
	}

	topicResults := make([]domain.TopicResult, len(results))
	for i, r := range results {
		topicResults[i] = domain.TopicResult{
			Code:       r.Code,
			Label:      r.Label,
			Confidence: r.Confidence,
		}
	}
	return &PhaseResult{
		PhaseName: p.Name(),
		Success:   true,
		Duration:  time.Since(start),
		Data: map[string]interface{}{
			"topics": topicResults,
		},
	}, nil
}

// SummarizationPhase – фаза суммаризации
type SummarizationPhase struct {
	config *config.Config
}

func NewSummarizationPhase(cfg *config.Config) *SummarizationPhase {
	return &SummarizationPhase{config: cfg}
}

func (p *SummarizationPhase) Name() string       { return "summarization" }
func (p *SummarizationPhase) Description() string { return "Extractive summarization" }

func (p *SummarizationPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()
	summarizer := summarization.New(summarization.Config{
		MaxSentences:    p.config.Summarization.MaxSentences,
		PositionWeight:  p.config.Summarization.PositionWeight,
		DiversityWeight: p.config.Summarization.DiversityWeight,
	})
	sentences := data.Transcript.Sentences
	var allMarkers []domain.Marker
	if ar, _ := data.PhaseResults["atomic_detection"]; ar != nil {
		if m, ok := ar.Data["markers"].([]domain.Marker); ok {
			allMarkers = append(allMarkers, m...)
		}
	}
	if cr, _ := data.PhaseResults["composite_analysis"]; cr != nil {
		if m, ok := cr.Data["composite_markers"].([]domain.Marker); ok {
			allMarkers = append(allMarkers, m...)
		}
	}
	summary := summarizer.Summarize(sentences, allMarkers)
	return &PhaseResult{
		PhaseName: p.Name(),
		Success:   true,
		Duration:  time.Since(start),
		Data: map[string]interface{}{
			"summary": summary,
		},
	}, nil
}

// EventExtractionPhase – фаза извлечения событий с персональным назначением
type EventExtractionPhase struct {
	config   *config.Config
	repo     *calendar.Repository
	profiles []participants.Profile
}

func NewEventExtractionPhase(cfg *config.Config) *EventExtractionPhase {
	return &EventExtractionPhase{config: cfg}
}

func (p *EventExtractionPhase) Name() string       { return "event_extraction" }
func (p *EventExtractionPhase) Description() string { return "Extract calendar events and assign to participants" }

func (p *EventExtractionPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()
	log.Println("DEBUG EventExtractionPhase: started")

	if p.repo == nil {
		repo, err := calendar.NewRepository(p.config.Calendar.DBPath)
		if err != nil {
			log.Printf("ERROR EventExtractionPhase: init repo: %v", err)
			return nil, fmt.Errorf("failed to init calendar repository: %w", err)
		}
		p.repo = repo
		log.Println("DEBUG EventExtractionPhase: repo initialized")
	}
	if p.profiles == nil {
		profiles, err := participants.LoadProfiles(p.config.Profiles.Path)
		if err != nil {
			log.Printf("ERROR EventExtractionPhase: load profiles: %v", err)
			return nil, fmt.Errorf("failed to load profiles: %w", err)
		}
		p.profiles = profiles
		log.Printf("DEBUG EventExtractionPhase: loaded %d profiles", len(profiles))
	}

	text := data.Transcript.ProcessedText
	log.Printf("DEBUG EventExtractionPhase: text length=%d", len(text))
	var markers []domain.Marker
	if ar, _ := data.PhaseResults["atomic_detection"]; ar != nil {
		if m, ok := ar.Data["markers"].([]domain.Marker); ok {
			markers = append(markers, m...)
		}
	}
	log.Printf("DEBUG EventExtractionPhase: markers collected: %d", len(markers))

	// Извлекаем персон из метаданных, сохранённых в AtomicDetectionPhase
	var customPersons []string
	if raw, ok := data.Transcript.Metadata["custom_persons"]; ok {
		if p, ok := raw.([]string); ok {
			customPersons = p
		}
	}
	log.Printf("DEBUG EventExtractionPhase: custom persons %d", len(customPersons))

	extractor := calendar.NewExtractor()
events := extractor.Extract(text, markers, data.Transcript.Sentences, customPersons)
log.Printf("DEBUG EventExtractionPhase: extracted %d events", len(events))
for i, ev := range events {
    log.Printf("DEBUG EventExtractionPhase: event[%d] title=%q start=%v", i, ev.Title, ev.Start)
}

for i := range events {
    events[i].TranscriptID = data.Transcript.ID
    // assignee уже установлен экстрактором по customPersons – оставляем его
    log.Printf("DEBUG EventExtractionPhase: event [%s] assignee=%q", events[i].Title, events[i].Assignee)
}

if err := p.repo.DeleteByTranscriptID(data.Transcript.ID); err != nil {
    log.Printf("WARNING EventExtractionPhase: delete old events: %v", err)
}

for _, ev := range events {
    id, err := p.repo.Insert(ev)
    if err != nil {
        log.Printf("ERROR EventExtractionPhase: insert event: %v", err)
    } else {
        log.Printf("DEBUG EventExtractionPhase: inserted event id=%d", id)
    }
}

log.Println("DEBUG EventExtractionPhase: finished")
return &PhaseResult{
    PhaseName: p.Name(),
    Success:   true,
    Duration:  time.Since(start),
    Data: map[string]interface{}{
        "events_extracted": len(events),
    },
}, nil
}

// findProfileForText ищет имя профиля в заданном тексте (регистронезависимо)
func findProfileForText(text string, profiles []participants.Profile) string {
	lower := strings.ToLower(text)
	for _, p := range profiles {
		if p.ID == "default" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(p.Name)) {
			return p.Name
		}
	}
	return ""
}

// ProtocolGenerationPhase – генерация протокола
type ProtocolGenerationPhase struct {
	config   *config.Config
	profiles []participants.Profile
}

func NewProtocolGenerationPhase(cfg *config.Config) *ProtocolGenerationPhase {
	return &ProtocolGenerationPhase{config: cfg}
}

func (p *ProtocolGenerationPhase) Name() string       { return "protocol_generation" }
func (p *ProtocolGenerationPhase) Description() string { return "Generate meeting minutes" }

func (p *ProtocolGenerationPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	if p.profiles == nil {
		profiles, err := participants.LoadProfiles(p.config.Profiles.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to load profiles: %w", err)
		}
		p.profiles = profiles
	}

	tmpResult := &domain.AnalysisResult{
		Transcript: data.Transcript,
		Markers:    []domain.Marker{},
		Summary:    "",
		Topics:     []domain.TopicResult{},
	}
	if ar, _ := data.PhaseResults["atomic_detection"]; ar != nil {
		if m, ok := ar.Data["markers"].([]domain.Marker); ok {
			tmpResult.Markers = append(tmpResult.Markers, m...)
		}
	}
	if cr, _ := data.PhaseResults["composite_analysis"]; cr != nil {
		if m, ok := cr.Data["composite_markers"].([]domain.Marker); ok {
			tmpResult.Markers = append(tmpResult.Markers, m...)
		}
	}
	if sr, _ := data.PhaseResults["semantic_analysis"]; sr != nil {
		if m, ok := sr.Data["semantic_markers"].([]domain.Marker); ok {
			tmpResult.Markers = append(tmpResult.Markers, m...)
		}
	}
	if sumRes, ok := data.PhaseResults["summarization"]; ok && sumRes != nil {
		if s, ok := sumRes.Data["summary"].(string); ok {
			tmpResult.Summary = s
		}
	}
	if clsRes, ok := data.PhaseResults["classification"]; ok && clsRes != nil {
		if topics, ok := clsRes.Data["topics"].([]domain.TopicResult); ok {
			tmpResult.Topics = topics
		}
	}

	generator := protocol.NewGenerator(p.profiles)
	protocolText := generator.Generate(tmpResult, data.Transcript.Source)

	protocolDir := p.config.Protocols.Dir
	os.MkdirAll(protocolDir, 0755)
	filename := filepath.Join(protocolDir, fmt.Sprintf("protocol_%s.md", time.Now().Format("20060102_150405")))
	if err := os.WriteFile(filename, []byte(protocolText), 0644); err != nil {
		return nil, fmt.Errorf("failed to write protocol: %w", err)
	}

	return &PhaseResult{
		PhaseName: p.Name(),
		Success:   true,
		Duration:  time.Since(start),
		Data: map[string]interface{}{
			"protocol_file": filename,
		},
	}, nil
}

// NotificationPhase – генерация уведомлений участникам
type NotificationPhase struct {
	config   *config.Config
	profiles []participants.Profile
}

func NewNotificationPhase(cfg *config.Config) *NotificationPhase {
	return &NotificationPhase{config: cfg}
}

func (p *NotificationPhase) Name() string       { return "notifications" }
func (p *NotificationPhase) Description() string { return "Generate personal notifications" }

func (p *NotificationPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	if p.profiles == nil {
		profiles, err := participants.LoadProfiles(p.config.Profiles.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to load profiles: %w", err)
		}
		p.profiles = profiles
	}

	var allMarkers []domain.Marker
	if ar, _ := data.PhaseResults["atomic_detection"]; ar != nil {
		if m, ok := ar.Data["markers"].([]domain.Marker); ok {
			allMarkers = append(allMarkers, m...)
		}
	}
	if cr, _ := data.PhaseResults["composite_analysis"]; cr != nil {
		if m, ok := cr.Data["composite_markers"].([]domain.Marker); ok {
			allMarkers = append(allMarkers, m...)
		}
	}
	if sr, _ := data.PhaseResults["semantic_analysis"]; sr != nil {
		if m, ok := sr.Data["semantic_markers"].([]domain.Marker); ok {
			allMarkers = append(allMarkers, m...)
		}
	}

	notifier := notifications.NewNotifier(p.config.Notifications.Dir)
	tmpResult := &domain.AnalysisResult{
		Transcript: data.Transcript,
		Markers:    allMarkers,
	}
	if err := notifier.Generate(tmpResult, nil); err != nil {
		return nil, fmt.Errorf("notification generation failed: %w", err)
	}

	return &PhaseResult{
		PhaseName: p.Name(),
		Success:   true,
		Duration:  time.Since(start),
		Data: map[string]interface{}{
			"notifications_dir": p.config.Notifications.Dir,
		},
	}, nil
}

// PostProcessingPhase – финальная обработка с фильтрацией негативных паттернов
type PostProcessingPhase struct {
	config      *config.Config
	seedManager *determinism.SeedManager
}

func NewPostProcessingPhase(cfg *config.Config, sm *determinism.SeedManager) *PostProcessingPhase {
	return &PostProcessingPhase{config: cfg, seedManager: sm}
}

func (p *PostProcessingPhase) Name() string       { return "post_processing" }
func (p *PostProcessingPhase) Description() string { return "Post-processing and finalization" }

func (p *PostProcessingPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	filterConfig := final.FilterConfig{
		SuppressionRules: convertSuppressionRules(p.config.FinalProcessing.SuppressionRules),
		Deduplication: final.DeduplicationConfig{
			Enabled:            p.config.FinalProcessing.Deduplication.Enabled,
			OverlapThreshold:   p.config.FinalProcessing.Deduplication.OverlapThreshold,
			MinConfidenceDelta: p.config.FinalProcessing.Deduplication.MinConfidenceDelta,
			Strategy:           p.config.FinalProcessing.Deduplication.Strategy,
		},
	}
	filterEngine := final.NewFilterEngine(filterConfig)

	var currentMarkers []domain.Marker
	if ar, _ := data.PhaseResults["atomic_detection"]; ar != nil {
		if m, ok := ar.Data["markers"].([]domain.Marker); ok {
			currentMarkers = append(currentMarkers, m...)
		}
	}
	if cr, _ := data.PhaseResults["composite_analysis"]; cr != nil {
		if m, ok := cr.Data["composite_markers"].([]domain.Marker); ok {
			currentMarkers = append(currentMarkers, m...)
		}
	}
	if sr, _ := data.PhaseResults["semantic_analysis"]; sr != nil {
		if m, ok := sr.Data["semantic_markers"].([]domain.Marker); ok {
			currentMarkers = append(currentMarkers, m...)
		}
	}

	filteredMarkers, _ := filterEngine.ApplySuppressionFilters(currentMarkers)

	if ar, _ := data.PhaseResults["atomic_detection"]; ar != nil {
		ar.Data["markers"] = filteredMarkers
	}

	return &PhaseResult{
		PhaseName:   p.Name(),
		Success:     true,
		Duration:    time.Since(start),
		AuditEvents: filterEngine.GetAuditEvents(),
		Data: map[string]interface{}{
			"processed":        true,
			"filtered_markers": len(filteredMarkers),
		},
	}, nil
}

// IndexingPhase – индексация результатов для полнотекстового поиска
type IndexingPhase struct {
	config  *config.Config
	indexer *search.Indexer
}

func NewIndexingPhase(cfg *config.Config) *IndexingPhase {
	return &IndexingPhase{config: cfg}
}

func (p *IndexingPhase) Name() string       { return "indexing" }
func (p *IndexingPhase) Description() string { return "Index analysis result for full-text search" }

func (p *IndexingPhase) Execute(ctx context.Context, data *PipelineData) (*PhaseResult, error) {
	start := time.Now()

	if p.indexer == nil {
		indexer, err := search.NewIndexer(p.config.Search.IndexPath)
		if err != nil {
			return nil, fmt.Errorf("failed to init search indexer: %w", err)
		}
		p.indexer = indexer
	}

	doc := search.Document{
		ID:     data.Transcript.ID,
		Text:   data.Transcript.ProcessedText,
		Source: data.Transcript.Source,
		Date:   time.Now(),
		Summary: func() string {
			if sr, ok := data.PhaseResults["summarization"]; ok && sr != nil {
				if s, ok := sr.Data["summary"].(string); ok {
					return s
				}
			}
			return ""
		}(),
	}

	if ar, _ := data.PhaseResults["atomic_detection"]; ar != nil {
		if markers, ok := ar.Data["markers"].([]domain.Marker); ok {
			for _, m := range markers {
				doc.Markers = append(doc.Markers, m.Type+":"+m.TextSpan)
			}
		}
	}
	if cr, ok := data.PhaseResults["classification"]; ok && cr != nil {
		if topics, ok := cr.Data["topics"].([]domain.TopicResult); ok {
			for _, t := range topics {
				doc.Topics = append(doc.Topics, t.Code)
			}
		}
	}

	if err := p.indexer.IndexDoc(doc); err != nil {
		return nil, fmt.Errorf("indexing failed: %w", err)
	}

	if err := p.indexer.Close(); err != nil {
		log.Printf("Warning: closing indexer after indexing: %v", err)
	}
	p.indexer = nil

	return &PhaseResult{
		PhaseName: p.Name(),
		Success:   true,
		Duration:  time.Since(start),
		Data: map[string]interface{}{
			"indexed": true,
		},
	}, nil
}

// --- Вспомогательные функции ---

func convertCandidatesToMarkers(candidates []composite.CompositeCandidate) []domain.Marker {
	var markers []domain.Marker
	for _, c := range candidates {
		m := domain.NewMarker(
			2,
			c.PrimaryType+"_COMPOSITE",
			fmt.Sprintf("%s (объединение %d маркеров)", c.PrimaryType, c.MarkerCount),
			c.SpanStart,
			c.SpanEnd,
			c.Score, // уже fixed-point
			"composite_engine",
			"",
			"",
			false,
		)
		m.AddMetadata("candidate_id", c.ID)
		m.AddMetadata("marker_count", c.MarkerCount)
		m.AddMetadata("density", c.Density)
		markers = append(markers, *m)
	}
	return markers
}

func convertSuppressionRules(rules []config.SuppressionRule) []final.SuppressionRule {
	out := make([]final.SuppressionRule, len(rules))
	for i, r := range rules {
		out[i] = final.SuppressionRule{
			ID:      r.ID,
			Name:    r.Name,
			Type:    r.Type,
			Pattern: r.Pattern,
			ApplyTo: r.ApplyTo,
			Reason:  r.Reason,
			Weight:  r.Weight,
			Enabled: r.Enabled,
		}
	}
	return out
}

func convertTopics(topics []config.TopicConfig) []classification.TopicConfig {
	out := make([]classification.TopicConfig, len(topics))
	for i, t := range topics {
		kws := make([]classification.KeywordWeight, len(t.Keywords))
		for j, kw := range t.Keywords {
			// Вес по умолчанию 1.0, если не задан
			w := kw.Weight
			if w == 0 {
				w = 1.0
			}
			kws[j] = classification.KeywordWeight{
				Word:   kw.Word,
				Weight: w,
			}
		}
		out[i] = classification.TopicConfig{
			Code:     t.Code,
			Label:    t.Label,
			Keywords: kws,
		}
	}
	return out
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}