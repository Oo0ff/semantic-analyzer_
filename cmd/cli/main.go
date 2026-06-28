package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"semantic-analyzer/cmd/pipeline"
	"semantic-analyzer/internal/calendar"
	"semantic-analyzer/internal/domain"
	"semantic-analyzer/internal/search"
	"semantic-analyzer/pkg/config"
	"semantic-analyzer/pkg/determinism"
)

func main() {
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	inputPath := flag.String("input", "", "Path to input file (text or WAV audio)")
	outputDir := flag.String("output", "./results", "Output directory for results")
	enablePipeline := flag.Bool("pipeline", true, "Enable full pipeline processing")
	useAudio := flag.Bool("audio", false, "Force treat input as audio file (overrides extension detection)")

	// Календарные команды
	calendarList := flag.Bool("calendar-list", false, "List all calendar events")
	calendarExport := flag.Bool("calendar-export", false, "Export calendar events to iCalendar (.ics) file")
	calendarExportFile := flag.String("calendar-export-file", "calendar_export.ics", "Output file for iCalendar export")

	// Поиск
	searchQuery := flag.String("search", "", "Search query for full-text search across transcripts")

	flag.Parse()

	// Загружаем конфигурацию (нужна и для анализа, и для календаря, и для поиска)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// === Режим поиска ===
	if *searchQuery != "" {
		if err := handleSearchCommand(cfg, *searchQuery); err != nil {
			log.Fatalf("Search failed: %v", err)
		}
		return
	}

	// === Режим календарных команд ===
	if *calendarList || *calendarExport {
		if err := handleCalendarCommands(cfg, *calendarList, *calendarExport, *calendarExportFile); err != nil {
			log.Fatalf("Calendar command failed: %v", err)
		}
		return
	}

	// === Обычный анализ ===
	if *inputPath == "" {
		log.Fatal("Input file path is required. Use --input flag.")
	}

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	if err := runAnalysis(cfg, *configPath, *inputPath, *outputDir, *enablePipeline, *useAudio); err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	log.Println("Analysis completed successfully")
}

// handleSearchCommand выполняет поисковый запрос и выводит результаты
func handleSearchCommand(cfg *config.Config, query string) error {
	indexer, err := search.NewIndexer(cfg.Search.IndexPath)
	if err != nil {
		return fmt.Errorf("failed to open search index: %w", err)
	}
	defer indexer.Close()

	results, err := indexer.Search(query, 10)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) > 0 && len(results[0].Hits) > 0 {
		fmt.Printf("Найдено %d результатов по запросу \"%s\":\n", results[0].Total, query)
		for _, hit := range results[0].Hits {
			fmt.Printf("- %s (источник: %s)\n", hit.ID, hit.Fields["source"])
			if fragments, ok := hit.Fragments["text"]; ok && len(fragments) > 0 {
				fmt.Printf("  %s\n", strings.Join(fragments, "..."))
			}
		}
	} else {
		fmt.Println("Ничего не найдено.")
	}
	return nil
}

// handleCalendarCommands выполняет календарные операции
func handleCalendarCommands(cfg *config.Config, list, export bool, exportFile string) error {
	repo, err := calendar.NewRepository(cfg.Calendar.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open calendar database: %w", err)
	}
	defer repo.Close()

	if list {
		events, err := repo.List(time.Time{}, time.Time{})
		if err != nil {
			return fmt.Errorf("failed to list events: %w", err)
		}
		if len(events) == 0 {
			fmt.Println("Нет запланированных событий.")
			return nil
		}
		fmt.Printf("Найдено %d событий:\n", len(events))
		for i, ev := range events {
			fmt.Printf("%d. [%s] %s\n", i+1, ev.Start.Format("2006-01-02 15:04"), ev.Title)
			if ev.Description != "" {
				fmt.Printf("   Описание: %s\n", ev.Description)
			}
			if len(ev.Attendees) > 0 {
				fmt.Printf("   Участники: %s\n", strings.Join(ev.Attendees, ", "))
			}
			fmt.Println()
		}
	}

	if export {
		events, err := repo.List(time.Time{}, time.Time{})
		if err != nil {
			return fmt.Errorf("failed to fetch events for export: %w", err)
		}
		icsData := calendar.ExportToICS(events)
		if err := os.WriteFile(exportFile, []byte(icsData), 0644); err != nil {
			return fmt.Errorf("failed to write iCalendar file: %w", err)
		}
		fmt.Printf("Календарь экспортирован в %s\n", exportFile)
	}

	return nil
}

func runAnalysis(cfg *config.Config, configPath, inputPath, outputDir string, enablePipeline, forceAudio bool) error {
	startTime := time.Now()

	log.Printf("Loading config from: %s", configPath)
	log.Printf("Audio enabled in config: %v", cfg.Audio.Enabled)
	log.Printf("Supported audio formats: %v", cfg.Audio.SupportedFormats)

	seedManager := determinism.NewSeedManager(cfg.Determinism.BaseSeed)

	var rawText string
	var sourceType = "text"

	isAudioInput := forceAudio
	log.Printf("Force audio (--audio flag): %v", forceAudio)
	if !isAudioInput {
		ext := strings.ToLower(filepath.Ext(inputPath))
		log.Printf("File extension (lower): %s", ext)
		for _, fmtExt := range cfg.Audio.SupportedFormats {
			if ext == fmtExt {
				isAudioInput = true
				break
			}
		}
	}
	log.Printf("Is audio input (final): %v", isAudioInput)

	if cfg.Audio.Enabled && isAudioInput {
		log.Printf("Audio input detected (or forced with --audio). Starting transcription via Python...")
		transcribed, err := transcribeAudioViaPython(inputPath)
		if err != nil {
			return fmt.Errorf("audio transcription failed: %w", err)
		}
		rawText = transcribed
		sourceType = "audio-transcribed"

		debugPath := filepath.Join(outputDir, fmt.Sprintf("raw_transcript_%s.txt", time.Now().Format("20060102_150405")))
		if err := os.WriteFile(debugPath, []byte(rawText), 0644); err != nil {
			log.Printf("Warning: could not save raw transcript for debug: %v", err)
		} else {
			log.Printf("Raw transcribed text saved for debug: %s", debugPath)
		}
	} else {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return fmt.Errorf("failed to read input file: %w", err)
		}
		rawText = string(data)
	}

	traceID := seedManager.DeterministicHash(rawText)

	transcript := &domain.Transcript{
		ID:        traceID,
		RawText:   rawText,
		Source:    filepath.Base(inputPath) + " (" + sourceType + ")",
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"file_size_bytes": len(rawText),
			"input_type":      sourceType,
			"transcribed":     isAudioInput,
		},
	}

	var result *domain.AnalysisResult
	var analysisErr error

	if enablePipeline {
		result, analysisErr = runPipelineAnalysis(cfg, transcript, seedManager)
	} else {
		result, analysisErr = runLegacyAnalysis(cfg, transcript, seedManager)
	}
	if analysisErr != nil {
		return fmt.Errorf("analysis phase failed: %w", analysisErr)
	}

	outputFile := filepath.Join(outputDir, fmt.Sprintf("analysis_%s_%s.json", result.Transcript.ID[:8], time.Now().Format("20060102_150405")))
	outputData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal analysis result to JSON: %w", err)
	}

	if err := os.WriteFile(outputFile, outputData, 0644); err != nil {
		return fmt.Errorf("failed to write output JSON file: %w", err)
	}

	printSummary(result, outputFile, time.Since(startTime))

	return nil
}

// transcribeAudioViaPython вызывает внешний Python-скрипт для транскрипции аудио (Vosk)
func transcribeAudioViaPython(filePath string) (string, error) {
	cmd := exec.Command("python", "transcribe.py", filePath)
	out, err := cmd.Output()
	if err != nil {
		// Если Python не в PATH, попробуем "python3"
		cmd2 := exec.Command("python3", "transcribe.py", filePath)
		out2, err2 := cmd2.Output()
		if err2 != nil {
			return "", fmt.Errorf("failed to run Python transcription script: %v (tried 'python' and 'python3')", err)
		}
		out = out2
	}
	text := strings.TrimSpace(string(out))
	if text == "" || strings.Contains(text, "no speech") {
		log.Println("WARNING: transcription returned empty or placeholder text")
		return "[speech not recognized]", nil
	}
	return text, nil
}

func runPipelineAnalysis(cfg *config.Config, transcript *domain.Transcript, seedManager *determinism.SeedManager) (*domain.AnalysisResult, error) {
	p := pipeline.NewPipeline(cfg)

	if err := p.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize pipeline: %w", err)
	}

	return p.Process(context.Background(), transcript)
}

func runLegacyAnalysis(cfg *config.Config, transcript *domain.Transcript, seedManager *determinism.SeedManager) (*domain.AnalysisResult, error) {
	return &domain.AnalysisResult{
		TranscriptID: transcript.ID,
		Transcript:   transcript,
		Markers:      []domain.Marker{},
		Statistics: domain.Statistics{
			TotalMarkers:      0,
			ProcessingTime:    0,
			AverageConfidence: 0,
		},
		Timestamp:  time.Now(),
		ConfigHash: seedManager.DeterministicHash(fmt.Sprintf("%v", cfg)),
	}, nil
}

func printSummary(result *domain.AnalysisResult, outputFile string, duration time.Duration) {
	fmt.Printf("\n=== Analysis Summary ===\n")
	fmt.Printf("Transcript ID: %s\n", result.TranscriptID[:12])
	fmt.Printf("Source: %s\n", result.Transcript.Source)
	fmt.Printf("Total words: %d\n", result.Statistics.WordCount)
	fmt.Printf("Total sentences: %d\n", result.Statistics.SentenceCount)
	fmt.Printf("Total markers: %d\n", result.Statistics.TotalMarkers)
	fmt.Printf("  Atomic markers: %d\n", result.Statistics.AtomicMarkers)
	fmt.Printf("  Composite markers: %d\n", result.Statistics.CompositeMarkers)
	fmt.Printf("Average confidence: %.2f\n", result.Statistics.AverageConfidence)
	fmt.Printf("Processing time: %v\n", duration)
	fmt.Printf("Output file: %s\n", outputFile)

	if len(result.Statistics.MarkerTypes) > 0 {
		fmt.Printf("\nMarker types:\n")
		for t, c := range result.Statistics.MarkerTypes {
			fmt.Printf("  %s: %d\n", t, c)
		}
	}
	fmt.Println()
}