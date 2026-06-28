package text

import (
    "bytes"
    "fmt"
    "regexp"
    "strings"
    "sync"
    "unicode"
    "unicode/utf8"

    "golang.org/x/text/transform"
    "golang.org/x/text/unicode/norm"
    "semantic-analyzer/pkg/config"
)

// Preprocessor handles text normalization and tokenization
type Preprocessor struct {
    config        config.ProcessingConfig
    normForm      norm.Form
    tokenPool     sync.Pool
    bufferPool    sync.Pool
    sentenceRegex *regexp.Regexp
    delimiterSet  map[rune]bool
    stopWordSet   map[string]bool
    initialized   bool
    mu            sync.RWMutex
    locale        string // language/locale (e.g., "en-US", "ru-RU")
}

// NewPreprocessor creates a new Preprocessor with the given configuration and locale
func NewPreprocessor(cfg config.ProcessingConfig, locale string) *Preprocessor {
    p := &Preprocessor{
        config: cfg,
        locale: locale,
        tokenPool: sync.Pool{
            New: func() interface{} {
                return make([]string, 0, 100)
            },
        },
        bufferPool: sync.Pool{
            New: func() interface{} {
                return &bytes.Buffer{}
            },
        },
        delimiterSet: make(map[rune]bool),
        stopWordSet:  make(map[string]bool),
    }

    p.initialize()
    return p
}

// initialize sets up the preprocessor
func (p *Preprocessor) initialize() {
    if p.initialized {
        return
    }

    p.mu.Lock()
    defer p.mu.Unlock()

    // Set up Unicode normalization form
    switch p.config.UnicodeNormalization {
    case "NFC":
        p.normForm = norm.NFC
    case "NFD":
        p.normForm = norm.NFD
    case "NFKC":
        p.normForm = norm.NFKC
    case "NFKD":
        p.normForm = norm.NFKD
    default:
        p.normForm = norm.NFC
    }

    // Build delimiter set for efficient lookup
    for _, delim := range p.config.SentenceDelimiters {
        for _, r := range delim {
            p.delimiterSet[r] = true
        }
    }

    // Build stop word set for efficient lookup
    for _, word := range p.config.StopWords {
        if p.config.EnableLowercase {
            p.stopWordSet[strings.ToLower(word)] = true
        } else {
            p.stopWordSet[word] = true
        }
    }

    // Compile sentence segmentation regex
    pattern := p.buildSentencePattern()
    p.sentenceRegex = regexp.MustCompile(pattern)

    p.initialized = true
}

// buildSentencePattern creates a regex pattern for sentence segmentation
func (p *Preprocessor) buildSentencePattern() string {
    if len(p.config.SentenceDelimiters) == 0 {
        return `[.!?]+`
    }

    var parts []string
    for _, delim := range p.config.SentenceDelimiters {
        // Escape regex special characters
        escaped := regexp.QuoteMeta(delim)
        parts = append(parts, escaped)
    }

    pattern := "[" + strings.Join(parts, "") + "]+"
    return pattern + `\s+`
}

// Normalize normalizes text using Unicode normalization and optional lowercasing
func (p *Preprocessor) Normalize(text string) string {
    p.mu.RLock()
    defer p.mu.RUnlock()

    // Apply Unicode normalization
    normalized, _, err := transform.String(p.normForm, text)
    if err != nil {
        // Fall back to original text if normalization fails
        normalized = text
    }

    // Apply lowercasing if enabled
    if p.config.EnableLowercase {
        normalized = strings.ToLower(normalized)
    }

    // Locale-specific normalizations
    if p.locale == "ru-RU" {
        // Replace Cyrillic 'ё' with 'е' (common normalization in Russian)
        normalized = strings.ReplaceAll(normalized, "ё", "е")
    }
    // Additional locale-specific rules can be added here

    // Remove extra whitespace if not preserving
    if !p.config.PreserveWhitespace {
        normalized = strings.Join(strings.Fields(normalized), " ")
    }

    return normalized
}

// Tokenize splits text into tokens using memory pools
func (p *Preprocessor) Tokenize(text string) []string {
    p.mu.RLock()
    defer p.mu.RUnlock()

    // Get token slice from pool
    tokens := p.tokenPool.Get().([]string)
    defer func() {
        // Clear slice and return to pool
        tokens = tokens[:0]
        p.tokenPool.Put(tokens)
    }()

    // Get buffer from pool for building tokens
    buf := p.bufferPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()
        p.bufferPool.Put(buf)
    }()

    // Normalize text first
    normalized := p.Normalize(text)

    // Tokenize with unicode awareness
    inToken := false
    for _, r := range normalized {
        if unicode.IsSpace(r) {
            if inToken {
                token := buf.String()
                if !p.isStopWord(token) {
                    tokens = append(tokens, token)
                }
                buf.Reset()
                inToken = false
            }
        } else if p.config.RemovePunctuation && unicode.IsPunct(r) {
            // Skip punctuation if configured
            continue
        } else {
            buf.WriteRune(r)
            inToken = true
        }
    }

    // Handle last token
    if inToken {
        token := buf.String()
        if !p.isStopWord(token) {
            tokens = append(tokens, token)
        }
    }

    // Return a copy to avoid pool corruption
    result := make([]string, len(tokens))
    copy(result, tokens)
    return result
}

// SegmentIntoSentences splits text into sentences
func (p *Preprocessor) SegmentIntoSentences(text string) []string {
    p.mu.RLock()
    defer p.mu.RUnlock()

    // Normalize text first
    normalized := p.Normalize(text)

    // Use regex-based segmentation
    matches := p.sentenceRegex.FindAllStringIndex(normalized, -1)

    var sentences []string
    start := 0

    for _, match := range matches {
        end := match[1]
        sentence := strings.TrimSpace(normalized[start:end])
        if sentence != "" && len(sentence) <= p.config.MaxSentenceLength {
            sentences = append(sentences, sentence)
        }
        start = end
    }

    // Handle remaining text
    if start < len(normalized) {
        sentence := strings.TrimSpace(normalized[start:])
        if sentence != "" && len(sentence) <= p.config.MaxSentenceLength {
            sentences = append(sentences, sentence)
        }
    }

    return sentences
}

// ValidateEncoding checks if text contains valid UTF-8
func (p *Preprocessor) ValidateEncoding(text string) (bool, string) {
    // Check if text is valid UTF-8
    if !utf8.ValidString(text) {
        return false, "Invalid UTF-8 encoding detected"
    }

    // Check for control characters (optional)
    for i, r := range text {
        if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
            return false, fmt.Sprintf("Invalid control character at position %d", i)
        }
    }

    return true, ""
}

// GetWordFrequency calculates word frequency in text
func (p *Preprocessor) GetWordFrequency(text string) map[string]int {
    tokens := p.Tokenize(text)
    frequency := make(map[string]int)

    for _, token := range tokens {
        frequency[token]++
    }

    return frequency
}

// RemoveStopWords removes stop words from tokens
func (p *Preprocessor) RemoveStopWords(tokens []string) []string {
    p.mu.RLock()
    defer p.mu.RUnlock()

    var result []string
    for _, token := range tokens {
        if !p.isStopWord(token) {
            result = append(result, token)
        }
    }
    return result
}

// isStopWord checks if a word is a stop word
func (p *Preprocessor) isStopWord(word string) bool {
    if len(p.stopWordSet) == 0 {
        return false
    }

    checkWord := word
    if p.config.EnableLowercase {
        checkWord = strings.ToLower(word)
    }

    _, exists := p.stopWordSet[checkWord]
    return exists
}

// GetCharacterNGrams extracts character n-grams from text
func (p *Preprocessor) GetCharacterNGrams(text string, n int) []string {
    if n <= 0 || len(text) < n {
        return []string{}
    }

    runes := []rune(text)
    var ngrams []string

    for i := 0; i <= len(runes)-n; i++ {
        ngram := string(runes[i : i+n])
        ngrams = append(ngrams, ngram)
    }

    return ngrams
}

// GetWordNGrams extracts word n-grams from text
func (p *Preprocessor) GetWordNGrams(text string, n int) []string {
    tokens := p.Tokenize(text)
    if n <= 0 || len(tokens) < n {
        return []string{}
    }

    var ngrams []string
    for i := 0; i <= len(tokens)-n; i++ {
        ngram := strings.Join(tokens[i:i+n], " ")
        ngrams = append(ngrams, ngram)
    }

    return ngrams
}

// CleanText removes unwanted characters and normalizes
func (p *Preprocessor) CleanText(text string) string {
    // Normalize first
    cleaned := p.Normalize(text)

    // Remove control characters (keep whitespace)
    var result strings.Builder
    for _, r := range cleaned {
        if !unicode.IsControl(r) || r == '\n' || r == '\r' || r == '\t' {
            result.WriteRune(r)
        }
    }

    return result.String()
}

// SplitIntoParagraphs splits text into paragraphs
func (p *Preprocessor) SplitIntoParagraphs(text string) []string {
    paragraphs := strings.Split(text, "\n\n")
    var result []string

    for _, para := range paragraphs {
        trimmed := strings.TrimSpace(para)
        if trimmed != "" {
            result = append(result, trimmed)
        }
    }

    return result
}

// GetTextStatistics returns various text statistics
func (p *Preprocessor) GetTextStatistics(text string) map[string]interface{} {
    tokens := p.Tokenize(text)
    sentences := p.SegmentIntoSentences(text)
    paragraphs := p.SplitIntoParagraphs(text)

    // Calculate average word length
    totalChars := 0
    for _, token := range tokens {
        totalChars += len([]rune(token))
    }

    avgWordLength := 0.0
    if len(tokens) > 0 {
        avgWordLength = float64(totalChars) / float64(len(tokens))
    }

    // Calculate average sentence length
    avgSentenceLength := 0.0
    if len(sentences) > 0 {
        avgSentenceLength = float64(len(tokens)) / float64(len(sentences))
    }

    return map[string]interface{}{
        "character_count":      len([]rune(text)),
        "word_count":           len(tokens),
        "sentence_count":       len(sentences),
        "paragraph_count":      len(paragraphs),
        "avg_word_length":      avgWordLength,
        "avg_sentence_length":  avgSentenceLength,
        "unique_words":         len(p.GetWordFrequency(text)),
    }
}