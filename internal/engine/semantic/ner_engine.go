package semantic

import (
    "fmt"
    "regexp"
    "sort"
    "strings"
    "sync"
    "time"

    "semantic-analyzer/internal/domain"
    "semantic-analyzer/pkg/config"
)

// FixedPointScale — from globals
const FixedPointScale = 1000000

// NEREngine performs named entity recognition
type NEREngine struct {
    config        config.NERConfig
    patterns      map[string]*regexp.Regexp
    personTitles  map[string]bool
    orgSuffixes   map[string]bool
    locationWords map[string]bool
    personDict    []string            // словарь полных имён для точного поиска
    auditEvents   []domain.AuditEvent
    mu            sync.RWMutex
    traceID       string // provenance
}

// NewNEREngine creates a new NER engine with trace ID
func NewNEREngine(cfg config.NERConfig, traceID string) (*NEREngine, error) {
    engine := &NEREngine{
        config:        cfg,
        patterns:      make(map[string]*regexp.Regexp),
        personTitles:  make(map[string]bool),
        orgSuffixes:   make(map[string]bool),
        locationWords: make(map[string]bool),
        personDict:    cfg.PersonDictionary, // загружаем словарь имён
        auditEvents:   []domain.AuditEvent{},
        traceID:       traceID,
    }

    // Initialize pattern sets
    for _, title := range cfg.PersonTitles {
        engine.personTitles[strings.ToLower(title)] = true
    }

    for _, suffix := range cfg.OrganizationSuffixes {
        engine.orgSuffixes[strings.ToLower(suffix)] = true
    }

    for _, word := range cfg.LocationKeywords {
        engine.locationWords[strings.ToLower(word)] = true
    }

    // Compile regex patterns
    if err := engine.compilePatterns(); err != nil {
        return nil, err
    }

    return engine, nil
}

// compilePatterns compiles all regex patterns
func (engine *NEREngine) compilePatterns() error {
    // Compile person patterns
    for _, patternConfig := range engine.config.PersonPatterns {
        compiled, err := regexp.Compile(patternConfig.Pattern)
        if err != nil {
            return fmt.Errorf("failed to compile person pattern %s: %w", patternConfig.ID, err)
        }
        engine.patterns[patternConfig.ID] = compiled
    }

    // Compile organization patterns
    for _, patternConfig := range engine.config.OrganizationPatterns {
        compiled, err := regexp.Compile(patternConfig.Pattern)
        if err != nil {
            return fmt.Errorf("failed to compile organization pattern %s: %w", patternConfig.ID, err)
        }
        engine.patterns[patternConfig.ID] = compiled
    }

    // Compile location patterns
    for i, pattern := range engine.config.LocationPatterns {
        compiled, err := regexp.Compile(pattern)
        if err != nil {
            return fmt.Errorf("failed to compile location pattern %d: %w", i, err)
        }
        engine.patterns[fmt.Sprintf("location_%d", i)] = compiled
    }

    return nil
}

// ExtractEntities extracts named entities from text
func (engine *NEREngine) ExtractEntities(text string) []Entity {
    startTime := time.Now()

    // Create audit event with trace ID
    auditEvent := domain.NewAuditEvent("ner_engine", "extract_entities")
    auditEvent.AddData("text_length", len(text))
    auditEvent.AddData("trace_id", engine.traceID)
    engine.addAuditEvent(auditEvent)

    var entities []Entity

    // Extract persons
    persons := engine.extractPersons(text)
    entities = append(entities, persons...)

    // Extract organizations
    organizations := engine.extractOrganizations(text)
    entities = append(entities, organizations...)

    // Extract locations
    locations := engine.extractLocations(text)
    entities = append(entities, locations...)

    // Remove overlapping entities (keep highest confidence)
    entities = engine.removeOverlappingEntities(entities)

    // Add context to entities
    for i := range entities {
        entities[i].SetContext(text, 50)
    }

    // Create completion audit event with trace ID
    endEvent := domain.NewAuditEvent("ner_engine", "entities_extracted")
    endEvent.AddData("entities_found", len(entities))
    endEvent.AddData("persons", len(persons))
    endEvent.AddData("organizations", len(organizations))
    endEvent.AddData("locations", len(locations))
    endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
    endEvent.AddData("trace_id", engine.traceID)
    endEvent.SetDuration(startTime)
    engine.addAuditEvent(endEvent)

    return entities
}

// extractPersons extracts person entities from text
func (engine *NEREngine) extractPersons(text string) []Entity {
    var entities []Entity

    // Method 1: Use regex patterns
    for patternID, pattern := range engine.patterns {
        if strings.HasPrefix(patternID, "person_") {
            matches := pattern.FindAllStringIndex(text, -1)
            for _, match := range matches {
                entity := NewEntity(
                    "PERSON",
                    text[match[0]:match[1]],
                    match[0],
                    match[1],
                    0.9, // High confidence for pattern matches
                    patternID,
                )
                entity.AddCategory("person")
                entities = append(entities, *entity)
            }
        }
    }

    // Method 2: Look for title + name patterns
    words := strings.Fields(text)
    for i := 0; i < len(words)-1; i++ {
        word := strings.ToLower(strings.Trim(words[i], ".,;:!?"))

        // Check if this word is a title
        if engine.personTitles[word] {
            // Look for capitalized names following the title
            if i+2 < len(words) {
                nextWord1 := strings.Trim(words[i+1], ".,;:!?")
                nextWord2 := strings.Trim(words[i+2], ".,;:!?")

                if isCapitalized(nextWord1) && isCapitalized(nextWord2) {
                    // Found a likely person name
                    fullName := words[i] + " " + nextWord1 + " " + nextWord2
                    start := strings.Index(text, fullName)
                    if start != -1 {
                        entity := NewEntity(
                            "PERSON",
                            fullName,
                            start,
                            start+len(fullName),
                            0.8, // Medium-high confidence
                            "title_pattern",
                        )
                        entity.Subtype = "named_person"
                        entity.AddCategory("person")
                        entities = append(entities, *entity)
                    }
                }
            }
        }
    }

    // Method 3: Поиск точных совпадений из словаря персон (регистронезависимый)
    lowerText := strings.ToLower(text)
    for _, fullName := range engine.personDict {
        if fullName == "" {
            continue
        }
        lowerName := strings.ToLower(fullName)
        // Ищем все вхождения полного имени
        start := 0
        for {
            idx := strings.Index(lowerText[start:], lowerName)
            if idx == -1 {
                break
            }
            absIdx := start + idx
            endIdx := absIdx + len(lowerName)
            // Убедимся, что это целое слово (границы текста или пробелы)
            leftOk := absIdx == 0 || isWordBoundary(lowerText[absIdx-1])
            rightOk := endIdx == len(lowerText) || isWordBoundary(lowerText[endIdx])
            if leftOk && rightOk {
                entity := NewEntity(
                    "PERSON",
                    text[absIdx:endIdx], // используем оригинальный текст с регистром
                    absIdx,
                    endIdx,
                    0.85, // высокая уверенность
                    "person_dictionary",
                )
                entity.AddCategory("person")
                entities = append(entities, *entity)
            }
            start = endIdx
        }
    }

    return entities
}

// isWordBoundary reports whether a byte is a word boundary (space, punctuation, etc.)
func isWordBoundary(b byte) bool {
    return b == ' ' || b == '\n' || b == '\t' || b == '.' || b == ',' || b == '!' || b == '?' || b == ':' || b == ';'
}

// extractOrganizations extracts organization entities from text
func (engine *NEREngine) extractOrganizations(text string) []Entity {
    var entities []Entity

    // Method 1: Use regex patterns
    for patternID, pattern := range engine.patterns {
        if strings.HasPrefix(patternID, "organization_") {
            matches := pattern.FindAllStringIndex(text, -1)
            for _, match := range matches {
                entity := NewEntity(
                    "ORGANIZATION",
                    text[match[0]:match[1]],
                    match[0],
                    match[1],
                    0.85, // High confidence for pattern matches
                    patternID,
                )
                entity.AddCategory("organization")
                entities = append(entities, *entity)
            }
        }
    }

    // Method 2: Look for organization suffixes
    words := strings.Fields(text)
    for i := 0; i < len(words); i++ {
        word := strings.ToLower(strings.Trim(words[i], ".,;:!?"))

        // Check if this word is an organization suffix
        if engine.orgSuffixes[word] {
            // Look backward for organization name
            orgWords := []string{}
            for j := i - 1; j >= 0 && j >= i-3; j-- {
                prevWord := strings.Trim(words[j], ".,;:!?")
                if isCapitalized(prevWord) || containsLettersAndSymbols(prevWord) {
                    orgWords = append([]string{prevWord}, orgWords...)
                } else {
                    break
                }
            }

            if len(orgWords) > 0 {
                // Add the suffix
                orgWords = append(orgWords, words[i])
                orgName := strings.Join(orgWords, " ")

                start := strings.Index(text, orgName)
                if start != -1 {
                    entity := NewEntity(
                        "ORGANIZATION",
                        orgName,
                        start,
                        start+len(orgName),
                        0.7, // Medium confidence
                        "suffix_pattern",
                    )
                    entity.Subtype = "company"
                    entity.AddCategory("organization")
                    entities = append(entities, *entity)
                }
            }
        }
    }

    return entities
}

// extractLocations extracts location entities from text
func (engine *NEREngine) extractLocations(text string) []Entity {
    var entities []Entity

    // Method 1: Use regex patterns
    for patternID, pattern := range engine.patterns {
        if strings.HasPrefix(patternID, "location_") {
            matches := pattern.FindAllStringIndex(text, -1)
            for _, match := range matches {
                entityText := text[match[0]:match[1]]

                // Check if this looks like a location
                if engine.isLikelyLocation(entityText) {
                    entity := NewEntity(
                        "LOCATION",
                        entityText,
                        match[0],
                        match[1],
                        0.8, // Medium-high confidence
                        patternID,
                    )
                    entity.AddCategory("location")
                    entities = append(entities, *entity)
                }
            }
        }
    }

    // Method 2: Look for location keywords with context
    words := strings.Fields(text)
    for i := 0; i < len(words); i++ {
        word := strings.ToLower(strings.Trim(words[i], ".,;:!?"))

        // Check if this word is a location keyword
        if engine.locationWords[word] {
            // Look for location name around the keyword
            locationName := engine.findLocationName(words, i)
            if locationName != "" {
                start := strings.Index(text, locationName)
                if start != -1 {
                    entity := NewEntity(
                        "LOCATION",
                        locationName,
                        start,
                        start+len(locationName),
                        0.6, // Medium confidence
                        "keyword_context",
                    )
                    entity.Subtype = engine.getLocationSubtype(word)
                    entity.AddCategory("location")
                    entities = append(entities, *entity)
                }
            }
        }
    }

    return entities
}

// isLikelyLocation checks if text is likely a location
func (engine *NEREngine) isLikelyLocation(text string) bool {
    // Check for common location patterns
    words := strings.Fields(text)

    if len(words) == 0 {
        return false
    }

    // Check if first word is capitalized (common for locations)
    if !isCapitalized(words[0]) {
        return false
    }

    // Check for location keywords in the text
    for _, word := range words {
        if engine.locationWords[strings.ToLower(word)] {
            return true
        }
    }

    // Check for common location suffixes
    lastWord := words[len(words)-1]
    lowerLast := strings.ToLower(lastWord)

    locationSuffixes := []string{"street", "st", "avenue", "ave", "road", "rd",
                                 "boulevard", "blvd", "city", "town", "state", "country"}

    for _, suffix := range locationSuffixes {
        if strings.HasSuffix(lowerLast, suffix) {
            return true
        }
    }

    return false
}

// findLocationName finds a location name around a keyword
func (engine *NEREngine) findLocationName(words []string, keywordIndex int) string {
    // Collect words before and after the keyword
    var locationWords []string

    // Look backward for capitalized words
    for i := keywordIndex - 1; i >= 0 && i >= keywordIndex-3; i-- {
        word := strings.Trim(words[i], ".,;:!?")
        if isCapitalized(word) || containsDigits(word) {
            locationWords = append([]string{word}, locationWords...)
        } else {
            break
        }
    }

    // Add the keyword
    locationWords = append(locationWords, strings.Trim(words[keywordIndex], ".,;:!?"))

    // Look forward for additional context
    for i := keywordIndex + 1; i < len(words) && i <= keywordIndex+2; i++ {
        word := strings.Trim(words[i], ".,;:!?")
        if isCapitalized(word) || containsDigits(word) {
            locationWords = append(locationWords, word)
        } else {
            break
        }
    }

    if len(locationWords) > 1 {
        return strings.Join(locationWords, " ")
    }

    return ""
}

// getLocationSubtype determines the location subtype based on keyword
func (engine *NEREngine) getLocationSubtype(keyword string) string {
    switch keyword {
    case "street", "avenue", "road", "boulevard":
        return "address"
    case "city", "town":
        return "city"
    case "state":
        return "state"
    case "country":
        return "country"
    default:
        return "generic"
    }
}

// removeOverlappingEntities removes overlapping entities, keeping the highest confidence one
func (engine *NEREngine) removeOverlappingEntities(entities []Entity) []Entity {
    if len(entities) <= 1 {
        return entities
    }

    // Sort by start position
    sort.Slice(entities, func(i, j int) bool {
        if entities[i].Start == entities[j].Start {
            return entities[i].End < entities[j].End
        }
        return entities[i].Start < entities[j].Start
    })

    var result []Entity
    current := entities[0]

    for i := 1; i < len(entities); i++ {
        next := entities[i]

        if current.Overlaps(&next) {
            // Keep entity with higher confidence
            if current.Confidence >= next.Confidence {
                // Keep current, skip next
                continue
            } else {
                // Replace current with next
                current = next
            }
        } else {
            result = append(result, current)
            current = next
        }
    }

    // Add the last entity
    result = append(result, current)

    return result
}

// addAuditEvent adds an audit event
func (engine *NEREngine) addAuditEvent(event *domain.AuditEvent) {
    engine.mu.Lock()
    defer engine.mu.Unlock()
    engine.auditEvents = append(engine.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (engine *NEREngine) GetAuditEvents() []domain.AuditEvent {
    engine.mu.RLock()
    defer engine.mu.RUnlock()
    return engine.auditEvents
}

// Helper functions
func isCapitalized(word string) bool {
    if len(word) == 0 {
        return false
    }
    firstRune := []rune(word)[0]
    return (firstRune >= 'A' && firstRune <= 'Z') ||
           (firstRune >= 'А' && firstRune <= 'Я') // Cyrillic support
}

func containsLettersAndSymbols(word string) bool {
    hasLetter := false
    for _, r := range word {
        if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
           (r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') {
            hasLetter = true
        } else if r == '&' || r == '.' {
            // Allow common organization symbols
            continue
        } else if r >= '0' && r <= '9' {
            // Allow digits
            continue
        } else {
            return false
        }
    }
    return hasLetter
}

func containsDigits(word string) bool {
    for _, r := range word {
        if r >= '0' && r <= '9' {
            return true
        }
    }
    return false
}