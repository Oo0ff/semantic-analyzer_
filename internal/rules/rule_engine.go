package rules

import (
    "encoding/csv"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strconv"
    "strings"
    "sync"

    "semantic-analyzer/pkg/config"
)

// RuleEngine manages and executes detection rules
type RuleEngine struct {
    regexRules   []RegexRule
    keywordRules []KeywordRule
    regexCache   map[string]*regexp.Regexp
    mu           sync.RWMutex
    initialized  bool
}

// RegexRule represents a regex-based detection rule
type RegexRule struct {
    ID      string
    Pattern string
    Weight  float64
    Type    string
    Level   int
    Name    string
    Regex   *regexp.Regexp
}

// KeywordRule represents a keyword-based detection rule
type KeywordRule struct {
    ID            string
    Keywords      []string
    Weight        float64
    Type          string
    Level         int
    CaseSensitive bool
    Name          string
}

// NewRuleEngine creates a new RuleEngine from configuration
func NewRuleEngine(atomicRules config.AtomicRulesConfig) (*RuleEngine, error) {
    re := &RuleEngine{
        regexRules:   []RegexRule{},
        keywordRules: []KeywordRule{},
        regexCache:   make(map[string]*regexp.Regexp),
    }

    if err := re.loadRules(atomicRules); err != nil {
        return nil, err
    }

    re.initialized = true
    return re, nil
}

// loadRules loads rules from configuration
func (re *RuleEngine) loadRules(atomicRules config.AtomicRulesConfig) error {
    re.mu.Lock()
    defer re.mu.Unlock()

    // Load regex rules
    for _, ruleConfig := range atomicRules.RegexPatterns {
        compiled, err := regexp.Compile(ruleConfig.Pattern)
        if err != nil {
            return fmt.Errorf("failed to compile regex pattern for rule %s: %w", ruleConfig.ID, err)
        }

        rule := RegexRule{
            ID:      ruleConfig.ID,
            Pattern: ruleConfig.Pattern,
            Weight:  ruleConfig.Weight,
            Type:    ruleConfig.Type,
            Level:   ruleConfig.Level,
            Name:    ruleConfig.Name,
            Regex:   compiled,
        }

        re.regexRules = append(re.regexRules, rule)
        re.regexCache[ruleConfig.ID] = compiled
    }

    // Load keyword rules
    for _, ruleConfig := range atomicRules.KeywordLists {
        rule := KeywordRule{
            ID:            ruleConfig.ID,
            Keywords:      ruleConfig.Keywords,
            Weight:        ruleConfig.Weight,
            Type:          ruleConfig.Type,
            Level:         ruleConfig.Level,
            CaseSensitive: ruleConfig.CaseSensitive,
            Name:          ruleConfig.Name,
        }

        re.keywordRules = append(re.keywordRules, rule)
    }

    re.sortRules()
    return nil
}

// LoadCustomDictionaries читает все CSV‑файлы из директории dir и добавляет их содержимое
// в виде отдельных KeywordRule (по одному на каждое слово/строку). Формат CSV:
// word,type[,weight]   – если вес не указан, используется 0.8.
// Пример строки: "Газпром",ORG,0.9
// Каждое правило получает ID = "custom_<номер строки>" и level = 1.
func (re *RuleEngine) LoadCustomDictionaries(dir string) error {
    re.mu.Lock()
    defer re.mu.Unlock()

    files, err := filepath.Glob(filepath.Join(dir, "*.csv"))
    if err != nil {
        return fmt.Errorf("failed to list CSV files in %s: %w", dir, err)
    }

    for _, file := range files {
        f, err := os.Open(file)
        if err != nil {
            return fmt.Errorf("cannot open %s: %w", file, err)
        }

        reader := csv.NewReader(f)
        reader.TrimLeadingSpace = true
        reader.Comment = '#' // строки, начинающиеся с '#', считаются комментариями

        records, err := reader.ReadAll()
        f.Close()
        if err != nil {
            return fmt.Errorf("failed to read CSV %s: %w", file, err)
        }

        for _, record := range records {
            if len(record) == 0 {
                continue
            }
            word := strings.TrimSpace(record[0])
            if word == "" {
                continue
            }
            markerType := "CUSTOM"
            if len(record) >= 2 {
                markerType = strings.TrimSpace(record[1])
                if markerType == "" {
                    markerType = "CUSTOM"
                }
            }
            weight := 0.8
            if len(record) >= 3 {
                if w, err := strconv.ParseFloat(strings.TrimSpace(record[2]), 64); err == nil {
                    weight = w
                }
            }

            // Создаём уникальный ID на основе слова и типа
            ruleID := fmt.Sprintf("custom_%s_%d", strings.ReplaceAll(word, " ", "_"), len(re.keywordRules)+1)

            // Добавляем правило с одним ключевым словом (можно было бы группировать, но для простоты – по одному)
            rule := KeywordRule{
                ID:            ruleID,
                Keywords:      []string{word},
                Weight:        weight,
                Type:          markerType,
                Level:         1,
                CaseSensitive: false,
                Name:          fmt.Sprintf("Пользовательское слово: %s", word),
            }
            re.keywordRules = append(re.keywordRules, rule)
        }
    }

    re.sortRules()
    return nil
}

// sortRules sorts rules by priority
func (re *RuleEngine) sortRules() {
    sort.Slice(re.regexRules, func(i, j int) bool {
        if re.regexRules[i].Level == re.regexRules[j].Level {
            return re.regexRules[i].Weight > re.regexRules[j].Weight
        }
        return re.regexRules[i].Level > re.regexRules[j].Level
    })

    sort.Slice(re.keywordRules, func(i, j int) bool {
        if re.keywordRules[i].Level == re.keywordRules[j].Level {
            return re.keywordRules[i].Weight > re.keywordRules[j].Weight
        }
        return re.keywordRules[i].Level > re.keywordRules[j].Level
    })
}

// GetRegexRules returns all regex rules
func (re *RuleEngine) GetRegexRules() []RegexRule {
    re.mu.RLock()
    defer re.mu.RUnlock()

    rules := make([]RegexRule, len(re.regexRules))
    copy(rules, re.regexRules)
    return rules
}

// GetKeywordRules returns all keyword rules
func (re *RuleEngine) GetKeywordRules() []KeywordRule {
    re.mu.RLock()
    defer re.mu.RUnlock()

    rules := make([]KeywordRule, len(re.keywordRules))
    copy(rules, re.keywordRules)
    return rules
}

// GetRuleByID returns a rule by its ID
func (re *RuleEngine) GetRuleByID(id string) (interface{}, bool) {
    re.mu.RLock()
    defer re.mu.RUnlock()

    for _, rule := range re.regexRules {
        if rule.ID == id {
            return rule, true
        }
    }

    for _, rule := range re.keywordRules {
        if rule.ID == id {
            return rule, true
        }
    }

    return nil, false
}

// GetRulesByType returns rules of a specific type
func (re *RuleEngine) GetRulesByType(ruleType string) ([]RegexRule, []KeywordRule) {
    re.mu.RLock()
    defer re.mu.RUnlock()

    var regexResults []RegexRule
    var keywordResults []KeywordRule

    for _, rule := range re.regexRules {
        if strings.EqualFold(rule.Type, ruleType) {
            regexResults = append(regexResults, rule)
        }
    }

    for _, rule := range re.keywordRules {
        if strings.EqualFold(rule.Type, ruleType) {
            keywordResults = append(keywordResults, rule)
        }
    }

    return regexResults, keywordResults
}

// GetRulesByLevel returns rules of a specific level
func (re *RuleEngine) GetRulesByLevel(level int) ([]RegexRule, []KeywordRule) {
    re.mu.RLock()
    defer re.mu.RUnlock()

    var regexResults []RegexRule
    var keywordResults []KeywordRule

    for _, rule := range re.regexRules {
        if rule.Level == level {
            regexResults = append(regexResults, rule)
        }
    }

    for _, rule := range re.keywordRules {
        if rule.Level == level {
            keywordResults = append(keywordResults, rule)
        }
    }

    return regexResults, keywordResults
}

// AddRegexRule adds a new regex rule
func (re *RuleEngine) AddRegexRule(rule RegexRule) error {
    re.mu.Lock()
    defer re.mu.Unlock()

    for _, existing := range re.regexRules {
        if existing.ID == rule.ID {
            return fmt.Errorf("rule with ID %s already exists", rule.ID)
        }
    }

    compiled, err := regexp.Compile(rule.Pattern)
    if err != nil {
        return fmt.Errorf("failed to compile regex pattern: %w", err)
    }

    rule.Regex = compiled
    re.regexRules = append(re.regexRules, rule)
    re.regexCache[rule.ID] = compiled

    re.sortRules()
    return nil
}

// AddKeywordRule adds a new keyword rule
func (re *RuleEngine) AddKeywordRule(rule KeywordRule) error {
    re.mu.Lock()
    defer re.mu.Unlock()

    for _, existing := range re.keywordRules {
        if existing.ID == rule.ID {
            return fmt.Errorf("rule with ID %s already exists", rule.ID)
        }
    }

    re.keywordRules = append(re.keywordRules, rule)
    re.sortRules()
    return nil
}

// RemoveRule removes a rule by ID
func (re *RuleEngine) RemoveRule(id string) bool {
    re.mu.Lock()
    defer re.mu.Unlock()

    removed := false

    for i, rule := range re.regexRules {
        if rule.ID == id {
            re.regexRules = append(re.regexRules[:i], re.regexRules[i+1:]...)
            delete(re.regexCache, id)
            removed = true
            break
        }
    }

    if !removed {
        for i, rule := range re.keywordRules {
            if rule.ID == id {
                re.keywordRules = append(re.keywordRules[:i], re.keywordRules[i+1:]...)
                removed = true
                break
            }
        }
    }

    return removed
}

// UpdateRegexRule updates an existing regex rule
func (re *RuleEngine) UpdateRegexRule(updatedRule RegexRule) error {
    re.mu.Lock()
    defer re.mu.Unlock()

    for i, rule := range re.regexRules {
        if rule.ID == updatedRule.ID {
            compiled, err := regexp.Compile(updatedRule.Pattern)
            if err != nil {
                return fmt.Errorf("failed to compile regex pattern: %w", err)
            }

            updatedRule.Regex = compiled
            re.regexRules[i] = updatedRule
            re.regexCache[updatedRule.ID] = compiled

            re.sortRules()
            return nil
        }
    }

    return fmt.Errorf("rule with ID %s not found", updatedRule.ID)
}

// UpdateKeywordRule updates an existing keyword rule
func (re *RuleEngine) UpdateKeywordRule(updatedRule KeywordRule) error {
    re.mu.Lock()
    defer re.mu.Unlock()

    for i, rule := range re.keywordRules {
        if rule.ID == updatedRule.ID {
            re.keywordRules[i] = updatedRule
            re.sortRules()
            return nil
        }
    }

    return fmt.Errorf("rule with ID %s not found", updatedRule.ID)
}

// GetRuleCount returns the number of rules
func (re *RuleEngine) GetRuleCount() (int, int) {
    re.mu.RLock()
    defer re.mu.RUnlock()

    return len(re.regexRules), len(re.keywordRules)
}

// GetRuleTypes returns all unique rule types
func (re *RuleEngine) GetRuleTypes() []string {
    re.mu.RLock()
    defer re.mu.RUnlock()

    typeSet := make(map[string]bool)

    for _, rule := range re.regexRules {
        typeSet[rule.Type] = true
    }

    for _, rule := range re.keywordRules {
        typeSet[rule.Type] = true
    }

    types := make([]string, 0, len(typeSet))
    for t := range typeSet {
        types = append(types, t)
    }

    sort.Strings(types)
    return types
}

// ValidateRules validates all rules
func (re *RuleEngine) ValidateRules() []error {
    re.mu.RLock()
    defer re.mu.RUnlock()

    var errors []error

    for _, rule := range re.regexRules {
        if rule.ID == "" {
            errors = append(errors, fmt.Errorf("regex rule has empty ID"))
        }
        if rule.Pattern == "" {
            errors = append(errors, fmt.Errorf("regex rule %s has empty pattern", rule.ID))
        }
        if rule.Weight < 0 || rule.Weight > 1 {
            errors = append(errors, fmt.Errorf("regex rule %s has invalid weight: %f", rule.ID, rule.Weight))
        }
        if rule.Level < 1 || rule.Level > 5 {
            errors = append(errors, fmt.Errorf("regex rule %s has invalid level: %d", rule.ID, rule.Level))
        }
    }

    for _, rule := range re.keywordRules {
        if rule.ID == "" {
            errors = append(errors, fmt.Errorf("keyword rule has empty ID"))
        }
        if len(rule.Keywords) == 0 {
            errors = append(errors, fmt.Errorf("keyword rule %s has no keywords", rule.ID))
        }
        if rule.Weight < 0 || rule.Weight > 1 {
            errors = append(errors, fmt.Errorf("keyword rule %s has invalid weight: %f", rule.ID, rule.Weight))
        }
        if rule.Level < 1 || rule.Level > 5 {
            errors = append(errors, fmt.Errorf("keyword rule %s has invalid level: %d", rule.ID, rule.Level))
        }
    }

    return errors
}

// ClearRules clears all rules
func (re *RuleEngine) ClearRules() {
    re.mu.Lock()
    defer re.mu.Unlock()

    re.regexRules = []RegexRule{}
    re.keywordRules = []KeywordRule{}
    re.regexCache = make(map[string]*regexp.Regexp)
}

// ExportRules exports rules to configuration format
func (re *RuleEngine) ExportRules() config.AtomicRulesConfig {
    re.mu.RLock()
    defer re.mu.RUnlock()

    var regexConfigs []config.RegexRuleConfig
    var keywordConfigs []config.KeywordRuleConfig

    for _, rule := range re.regexRules {
        regexConfigs = append(regexConfigs, config.RegexRuleConfig{
            ID:      rule.ID,
            Pattern: rule.Pattern,
            Weight:  rule.Weight,
            Type:    rule.Type,
            Level:   rule.Level,
            Name:    rule.Name,
        })
    }

    for _, rule := range re.keywordRules {
        keywordConfigs = append(keywordConfigs, config.KeywordRuleConfig{
            ID:            rule.ID,
            Keywords:      rule.Keywords,
            Weight:        rule.Weight,
            Type:          rule.Type,
            Level:         rule.Level,
            CaseSensitive: rule.CaseSensitive,
            Name:          rule.Name,
        })
    }

    return config.AtomicRulesConfig{
        RegexPatterns: regexConfigs,
        KeywordLists:  keywordConfigs,
    }
}

// GetCompiledRegex returns compiled regex by ID
func (re *RuleEngine) GetCompiledRegex(id string) (*regexp.Regexp, bool) {
    re.mu.RLock()
    defer re.mu.RUnlock()

    regex, exists := re.regexCache[id]
    return regex, exists
}

// ReloadRules reloads rules from configuration
func (re *RuleEngine) ReloadRules(atomicRules config.AtomicRulesConfig) error {
    re.mu.Lock()
    defer re.mu.Unlock()

    re.ClearRules()
    return re.loadRules(atomicRules)
}

// GetPersonNames возвращает все ключевые слова правил с типом PERSON (в нижнем регистре).
func (re *RuleEngine) GetPersonNames() []string {
	re.mu.RLock()
	defer re.mu.RUnlock()
	names := make([]string, 0)
	for _, rule := range re.keywordRules {
		if strings.EqualFold(rule.Type, "PERSON") {
			for _, kw := range rule.Keywords {
				names = append(names, strings.ToLower(kw))
			}
		}
	}
	return names
}