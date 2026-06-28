package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Main configuration structure
type Config struct {
	App             AppConfig             `mapstructure:"app"`
	Determinism     DeterminismConfig     `mapstructure:"determinism"`
	Processing      ProcessingConfig      `mapstructure:"processing"`
	Rules           RulesConfig           `mapstructure:"rules"`
	Performance     PerformanceConfig     `mapstructure:"performance"`
	Output          OutputConfig          `mapstructure:"output"`
	Audio           AudioConfig           `mapstructure:"audio"`
	Calendar        CalendarConfig        `mapstructure:"calendar"`
	Summarization   SummarizationConfig   `mapstructure:"summarization"`
	Classification  ClassificationConfig  `mapstructure:"classification"`
	FinalProcessing FinalProcessingConfig `mapstructure:"final_processing"`
	Profiles        ProfilesConfig        `mapstructure:"profiles"`
	Protocols       DirectoryConfig       `mapstructure:"protocols"`
	Notifications   DirectoryConfig       `mapstructure:"notifications"`
	Search          SearchConfig          `mapstructure:"search"`
}

// App configuration
type AppConfig struct {
	Name        string `mapstructure:"name"`
	Version     string `mapstructure:"version"`
	MaxTextSize int    `mapstructure:"max_text_size"`
	LogLevel    string `mapstructure:"log_level"`
	EnableDebug bool   `mapstructure:"enable_debug"`
}

// Determinism configuration
type DeterminismConfig struct {
	BaseSeed         int64  `mapstructure:"base_seed"`
	EnableStableSort bool   `mapstructure:"enable_stable_sort"`
	HashAlgorithm    string `mapstructure:"hash_algorithm"`
}

// Text processing configuration
type ProcessingConfig struct {
	UnicodeNormalization string   `mapstructure:"unicode_normalization"`
	EnableLowercase      bool     `mapstructure:"enable_lowercase"`
	SentenceDelimiters   []string `mapstructure:"sentence_delimiters"`
	PreserveWhitespace   bool     `mapstructure:"preserve_whitespace"`
	MaxSentenceLength    int      `mapstructure:"max_sentence_length"`
	RemovePunctuation    bool     `mapstructure:"remove_punctuation"`
	StopWords            []string `mapstructure:"stop_words"`
	Locale               string   `mapstructure:"locale"` // язык/локаль, например "ru-RU"
}

// Rules configuration
type RulesConfig struct {
	Atomic    AtomicRulesConfig    `mapstructure:"atomic"`
	Composite CompositeRulesConfig `mapstructure:"composite"`
	Semantic  SemanticRulesConfig  `mapstructure:"semantic"`
}

type RegexRuleConfig struct {
	ID      string  `mapstructure:"id" yaml:"id"`
	Pattern string  `mapstructure:"pattern" yaml:"pattern"`
	Weight  float64 `mapstructure:"weight" yaml:"weight"`
	Type    string  `mapstructure:"type" yaml:"type"`
	Level   int     `mapstructure:"level" yaml:"level"`
	Name    string  `yaml:"name,omitempty"`
}

type KeywordRuleConfig struct {
	ID            string   `mapstructure:"id" yaml:"id"`
	Keywords      []string `mapstructure:"keywords" yaml:"keywords"`
	Weight        float64  `mapstructure:"weight" yaml:"weight"`
	Type          string   `mapstructure:"type" yaml:"type"`
	Level         int      `mapstructure:"level" yaml:"level"`
	CaseSensitive bool     `mapstructure:"case_sensitive,omitempty" yaml:"case_sensitive,omitempty"`
	Name          string   `yaml:"name,omitempty"`
}

type AtomicRulesConfig struct {
	RegexPatterns []RegexRuleConfig   `mapstructure:"regex_patterns" yaml:"regex_patterns"`
	KeywordLists  []KeywordRuleConfig `mapstructure:"keyword_lists" yaml:"keyword_lists"`
}

type CompositeRulesConfig struct {
	ProximityWindow  int                `mapstructure:"proximity_window" yaml:"proximity_window"`
	MinDensity       float64            `mapstructure:"min_density" yaml:"min_density"`
	MaxSpan          int                `mapstructure:"max_span" yaml:"max_span"`
	MaxCandidates    int                `mapstructure:"max_candidates" yaml:"max_candidates"`
	SelectionWeights SelectionWeights   `mapstructure:"selection_weights" yaml:"selection_weights"`
	TypeWeights      map[string]float64 `mapstructure:"type_weights" yaml:"type_weights"`
}

type SelectionWeights struct {
	MarkerCount     float64 `mapstructure:"marker_count" yaml:"marker_count"`
	Density         float64 `mapstructure:"density" yaml:"density"`
	SpanCompactness float64 `mapstructure:"span_compactness" yaml:"span_compactness"`
	TypeDiversity   float64 `mapstructure:"type_diversity" yaml:"type_diversity"`
}

type SemanticRulesConfig struct {
	NER              NERConfig           `mapstructure:"ner" yaml:"ner"`
	Networks         map[string][]string `mapstructure:"networks" yaml:"networks"`
	ProximityWeights ProximityWeights    `mapstructure:"proximity_weights" yaml:"proximity_weights"`
	LogicalRules     []LogicalRuleConfig `mapstructure:"logical_rules" yaml:"logical_rules"`
}

type NERConfig struct {
	PersonTitles         []string          `mapstructure:"person_titles" yaml:"person_titles"`
	OrganizationSuffixes []string          `mapstructure:"organization_suffixes" yaml:"organization_suffixes"`
	LocationPatterns     []string          `mapstructure:"location_patterns" yaml:"location_patterns"`
	PersonPatterns       []RegexRuleConfig `mapstructure:"person_patterns" yaml:"person_patterns"`
	OrganizationPatterns []RegexRuleConfig `mapstructure:"organization_patterns" yaml:"organization_patterns"`
	LocationKeywords     []string          `mapstructure:"location_keywords" yaml:"location_keywords"`
	PersonDictionary     []string          `mapstructure:"person_dictionary" yaml:"person_dictionary"`
}

type ProximityWeights struct {
	NetworkMatch   float64 `mapstructure:"network_match" yaml:"network_match"`
	TypeSimilarity float64 `mapstructure:"type_similarity" yaml:"type_similarity"`
	ContextOverlap float64 `mapstructure:"context_overlap" yaml:"context_overlap"`
}

type LogicalRuleConfig struct {
	ID         string  `mapstructure:"id" yaml:"id"`
	Expression string  `mapstructure:"expression" yaml:"expression"`
	Type       string  `mapstructure:"type" yaml:"type"`
	Level      int     `mapstructure:"level" yaml:"level"`
	Weight     float64 `mapstructure:"weight" yaml:"weight"`
	OutputType string  `mapstructure:"output_type" yaml:"output_type"`
}

// Performance configuration
type PerformanceConfig struct {
	MaxWorkers                int  `mapstructure:"max_workers"`
	BufferSize                int  `mapstructure:"buffer_size"`
	EnableMemoryPool          bool `mapstructure:"enable_memory_pool"`
	RegexCompilationCacheSize int  `mapstructure:"regex_compilation_cache_size"`
	EnableParallelProcessing  bool `mapstructure:"enable_parallel_processing"`
}

// Output configuration
type OutputConfig struct {
	EnableAuditTrail  bool   `mapstructure:"enable_audit_trail"`
	PrettyPrintJSON   bool   `mapstructure:"pretty_print_json"`
	IncludeConfidence bool   `mapstructure:"include_confidence_scores"`
	TimestampFormat   string `mapstructure:"timestamp_format"`
	OutputDirectory   string `mapstructure:"output_directory"`
	EnableCompression bool   `mapstructure:"enable_compression"`
	MaxMarkersPerFile int    `mapstructure:"max_markers_per_file"`
}

// Audio configuration
type AudioConfig struct {
	Enabled          bool     `mapstructure:"enabled"`
	ModelPath        string   `mapstructure:"model_path"`
	SampleRate       int      `mapstructure:"sample_rate"`
	SupportedFormats []string `mapstructure:"supported_formats"`
	PythonPath       string   `mapstructure:"python_path"`
	TranscribeScript string   `mapstructure:"transcribe_script"`
	FFmpegPath       string   `mapstructure:"ffmpeg_path"`
}

// Calendar configuration
type CalendarConfig struct {
	DBPath string `mapstructure:"db_path"`
}

// SummarizationConfig – параметры экстрактивной суммаризации
type SummarizationConfig struct {
	MaxSentences    int     `mapstructure:"max_sentences"`
	PositionWeight  float64 `mapstructure:"position_weight"`
	DiversityWeight float64 `mapstructure:"diversity_weight"` // вес MMR, по умолчанию 0.3
}

// ClassificationConfig – конфигурация тематической классификации
type ClassificationConfig struct {
	Topics        []TopicConfig `mapstructure:"topics"`
	MinConfidence float64       `mapstructure:"min_confidence"`
}

// TopicConfig – описание темы (код, название, ключевые слова с весами)
type TopicConfig struct {
	Code     string          `mapstructure:"code"`
	Label    string          `mapstructure:"label"`
	Keywords []KeywordWeight `mapstructure:"keywords"` // список слов с весами
}

// KeywordWeight – ключевое слово темы с опциональным весом
type KeywordWeight struct {
	Word   string  `mapstructure:"word"`   // само слово или фраза
	Weight float64 `mapstructure:"weight"` // вес (по умолчанию 1.0)
}

// FinalProcessingConfig – финальная обработка и фильтрация
type FinalProcessingConfig struct {
	SuppressionRules []SuppressionRule `mapstructure:"suppression_rules"`
	Deduplication    DeduplicationCfg  `mapstructure:"deduplication"`
}

// SuppressionRule – правило подавления ложного маркера
type SuppressionRule struct {
	ID      string   `mapstructure:"id"`
	Name    string   `mapstructure:"name"`
	Type    string   `mapstructure:"type"` // regex, keyword, pattern
	Pattern string   `mapstructure:"pattern"`
	ApplyTo []string `mapstructure:"apply_to"`
	Reason  string   `mapstructure:"reason"`
	Weight  float64  `mapstructure:"weight"`
	Enabled bool     `mapstructure:"enabled"`
}

// DeduplicationCfg – параметры дедупликации
type DeduplicationCfg struct {
	Enabled            bool    `mapstructure:"enabled"`
	OverlapThreshold   float64 `mapstructure:"overlap_threshold"`
	MinConfidenceDelta float64 `mapstructure:"min_confidence_delta"`
	Strategy           string  `mapstructure:"strategy"`
}

// ProfilesConfig – путь к файлу профилей участников
type ProfilesConfig struct {
	Path string `mapstructure:"path"`
}

// DirectoryConfig – обобщённая конфигурация директории
type DirectoryConfig struct {
	Dir string `mapstructure:"dir"`
}

// SearchConfig – настройки полнотекстового поиска
type SearchConfig struct {
	IndexPath string `mapstructure:"index_path"`
}

// Load configuration
func Load(configPath string) (*Config, error) {
	viper.SetConfigType("yaml")

	setDefaults()

	if configPath != "" {
		absPath, err := filepath.Abs(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path: %w", err)
		}

		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("config file does not exist: %s", absPath)
		}

		viper.SetConfigFile(absPath)
	} else {
		viper.AddConfigPath(".")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath("/etc/semantic-analyzer")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("SEMANTIC")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("Warning: Config file not found, using defaults only")
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// validateConfig performs basic validation
func validateConfig(cfg *Config) error {
	if cfg.App.MaxTextSize <= 0 {
		return fmt.Errorf("app.max_text_size must be positive")
	}

	if cfg.Processing.MaxSentenceLength <= 0 {
		return fmt.Errorf("processing.max_sentence_length must be positive")
	}
	if len(cfg.Processing.SentenceDelimiters) == 0 {
		return fmt.Errorf("processing.sentence_delimiters cannot be empty")
	}

	for i, rule := range cfg.Rules.Atomic.RegexPatterns {
		if rule.ID == "" {
			return fmt.Errorf("rules.atomic.regex_patterns[%d].id cannot be empty", i)
		}
		if rule.Pattern == "" {
			return fmt.Errorf("rules.atomic.regex_patterns[%d].pattern cannot be empty", i)
		}
		if rule.Weight < 0 || rule.Weight > 1 {
			return fmt.Errorf("rules.atomic.regex_patterns[%d].weight must be [0..1]", i)
		}
	}

	for i, list := range cfg.Rules.Atomic.KeywordLists {
		if list.ID == "" {
			return fmt.Errorf("rules.atomic.keyword_lists[%d].id cannot be empty", i)
		}
		if len(list.Keywords) == 0 {
			return fmt.Errorf("rules.atomic.keyword_lists[%d].keywords cannot be empty", i)
		}
	}

	if cfg.Rules.Composite.ProximityWindow <= 0 {
		return fmt.Errorf("rules.composite.proximity_window must be positive")
	}

	if cfg.Audio.Enabled {
		if cfg.Audio.ModelPath == "" {
			return fmt.Errorf("audio.model_path is required when audio.enabled=true")
		}
		if cfg.Audio.SampleRate <= 0 {
			return fmt.Errorf("audio.sample_rate must be positive")
		}
		if len(cfg.Audio.SupportedFormats) == 0 {
			return fmt.Errorf("audio.supported_formats cannot be empty when audio enabled")
		}
	}

	return nil
}

// setDefaults sets default values
func setDefaults() {
	viper.SetDefault("app.name", "Semantic Analyzer")
	viper.SetDefault("app.version", "1.0.0")
	viper.SetDefault("app.max_text_size", 1000000)
	viper.SetDefault("determinism.base_seed", 42)
	viper.SetDefault("processing.unicode_normalization", "NFC")
	viper.SetDefault("processing.enable_lowercase", true)
	viper.SetDefault("processing.sentence_delimiters", []string{".", "!", "?", ";", ":"})
	viper.SetDefault("processing.locale", "en")
	viper.SetDefault("audio.python_path", "python")
	viper.SetDefault("audio.transcribe_script", "../transcribe.py")

	// Новые значения по умолчанию
	viper.SetDefault("summarization.max_sentences", 3)
	viper.SetDefault("summarization.position_weight", 0.1)
	viper.SetDefault("summarization.diversity_weight", 0.3)  // добавлено
	viper.SetDefault("classification.min_confidence", 0.2)
	viper.SetDefault("final_processing.deduplication.enabled", false)
	viper.SetDefault("final_processing.deduplication.overlap_threshold", 0.7)
	viper.SetDefault("final_processing.deduplication.min_confidence_delta", 0.1)
	viper.SetDefault("final_processing.deduplication.strategy", "keep_highest")

	// Календарь
	viper.SetDefault("calendar.db_path", "./calendar.db")

	// Профили, протоколы, уведомления
	viper.SetDefault("profiles.path", "./profiles.yaml")
	viper.SetDefault("protocols.dir", "./protocols")
	viper.SetDefault("notifications.dir", "./notifications")

	// Поиск
	viper.SetDefault("search.index_path", "./search_index")

	viper.SetDefault("audio.enabled", false)
	viper.SetDefault("audio.model_path", "./models/vosk-model-small-en-us-0.15")
	viper.SetDefault("audio.sample_rate", 16000)
	viper.SetDefault("audio.supported_formats", []string{".wav"})
}