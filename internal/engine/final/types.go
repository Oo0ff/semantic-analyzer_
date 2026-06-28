package final

import "semantic-analyzer/internal/domain"

// FilterConfig holds configuration for filtering rules
type FilterConfig struct {
    SuppressionRules  []SuppressionRule  `json:"suppression_rules" yaml:"suppression_rules"`
    Deduplication     DeduplicationConfig `json:"deduplication" yaml:"deduplication"`
    ConflictResolution ConflictResolutionConfig `json:"conflict_resolution" yaml:"conflict_resolution"`
    Output            OutputConfig       `json:"output" yaml:"output"`
}

// SuppressionRule defines rules for suppressing false positives
type SuppressionRule struct {
    ID          string   `json:"id" yaml:"id"`
    Name        string   `json:"name" yaml:"name"`
    Type        string   `json:"type" yaml:"type"` // "regex", "keyword", "pattern"
    Pattern     string   `json:"pattern" yaml:"pattern"`
    ApplyTo     []string `json:"apply_to" yaml:"apply_to"` // Marker types to apply to
    Reason      string   `json:"reason" yaml:"reason"`
    Weight      float64  `json:"weight" yaml:"weight"`
    Enabled     bool     `json:"enabled" yaml:"enabled"`
}

// DeduplicationConfig holds deduplication settings
type DeduplicationConfig struct {
    Enabled            bool    `json:"enabled" yaml:"enabled"`
    OverlapThreshold   float64 `json:"overlap_threshold" yaml:"overlap_threshold"`
    MinConfidenceDelta float64 `json:"min_confidence_delta" yaml:"min_confidence_delta"`
    Strategy           string  `json:"strategy" yaml:"strategy"` // "keep_highest", "merge", "keep_earliest"
}

// ConflictResolutionConfig holds conflict resolution settings
type ConflictResolutionConfig struct {
    Enabled      bool    `json:"enabled" yaml:"enabled"`
    Priorities   []string `json:"priorities" yaml:"priorities"` // Order of priority fields
    TieBreaker   string  `json:"tie_breaker" yaml:"tie_breaker"` // "random_seed", "position", "rule_id"
}

// OutputConfig holds output formatting settings
type OutputConfig struct {
    Format           string            `json:"format" yaml:"format"` // "json", "yaml", "text"
    PrettyPrint      bool              `json:"pretty_print" yaml:"pretty_print"`
    IncludeAudit     bool              `json:"include_audit" yaml:"include_audit"`
    IncludeMetadata  bool              `json:"include_metadata" yaml:"include_metadata"`
    Fields           []string          `json:"fields" yaml:"fields"` // Fields to include
    FieldMappings    map[string]string `json:"field_mappings" yaml:"field_mappings"`
    TimestampFormat  string            `json:"timestamp_format" yaml:"timestamp_format"`
}

// Conflict represents a conflict between markers
type Conflict struct {
    Marker1       domain.Marker `json:"marker1"`
    Marker2       domain.Marker `json:"marker2"`
    Type          string        `json:"type"` // "overlap", "contradiction", "redundancy"
    OverlapRatio  float64       `json:"overlap_ratio"`
    Severity      float64       `json:"severity"`
    Resolution    string        `json:"resolution,omitempty"`
    WinnerID      string        `json:"winner_id,omitempty"`
    Reason        string        `json:"reason,omitempty"`
}

// FilterResult represents the result of filtering operations
type FilterResult struct {
    InputCount     int              `json:"input_count"`
    OutputCount    int              `json:"output_count"`
    Suppressed     []SuppressedItem `json:"suppressed,omitempty"`
    Deduplicated   []DeduplicatedItem `json:"deduplicated,omitempty"`
    Conflicts      []Conflict       `json:"conflicts,omitempty"`
    Resolved       []ResolvedItem   `json:"resolved,omitempty"`
    Statistics     FilterStatistics `json:"statistics"`
}

// SuppressedItem represents a suppressed marker
type SuppressedItem struct {
    MarkerID string `json:"marker_id"`
    RuleID   string `json:"rule_id"`
    Reason   string `json:"reason"`
    Type     string `json:"type"`
}

// DeduplicatedItem represents a deduplicated marker
type DeduplicatedItem struct {
    KeptMarkerID   string `json:"kept_marker_id"`
    RemovedMarkerID string `json:"removed_marker_id"`
    Reason         string `json:"reason"`
    OverlapRatio   float64 `json:"overlap_ratio"`
}

// ResolvedItem represents a resolved conflict
type ResolvedItem struct {
    ConflictID string `json:"conflict_id"`
    WinnerID   string `json:"winner_id"`
    LoserID    string `json:"loser_id"`
    Reason     string `json:"reason"`
    RuleApplied string `json:"rule_applied"`
}

// FilterStatistics holds statistics about filtering
type FilterStatistics struct {
    SuppressionCount  int     `json:"suppression_count"`
    DeduplicationCount int    `json:"deduplication_count"`
    ConflictCount     int     `json:"conflict_count"`
    TimeTakenMs       float64 `json:"time_taken_ms"`
    MemoryReductionKB float64 `json:"memory_reduction_kb,omitempty"`
}