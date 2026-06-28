package semantic

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"
	"strings"

	
)

// Entity представляет распознанную именованную сущность в тексте
type Entity struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"` // PERSON, ORGANIZATION, LOCATION, DATE и т.д.
	Subtype      string                 `json:"subtype,omitempty"`
	Text         string                 `json:"text"`
	Start        int                    `json:"start"`
	End          int                    `json:"end"`
	Confidence   float64                `json:"confidence"`
	RuleID       string                 `json:"rule_id,omitempty"`       // из какой атомарной/композитной правила взята
	SourceRuleID string                 `json:"source_rule_id,omitempty"` // альтернативное поле, если нужно
	Context      string                 `json:"context,omitempty"`
	SentenceID   int                    `json:"sentence_id,omitempty"`
	DetectedAt   time.Time              `json:"detected_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Aliases      []string               `json:"aliases,omitempty"`
	Categories   []string               `json:"categories,omitempty"`
	SubEntities  []Entity               `json:"sub_entities,omitempty"`
	IsAtomic     bool                   `json:"is_atomic"`
}

// NewEntity создаёт новую сущность
func NewEntity(entityType, text string, start, end int, confidence float64, ruleID string) *Entity {
	return &Entity{
		ID:           generateEntityID(text, start, end, entityType),
		Type:         entityType,
		Text:         text,
		Start:        start,
		End:          end,
		Confidence:   confidence,
		RuleID:       ruleID,
		SourceRuleID: ruleID, // можно сделать отдельно, если нужно
		DetectedAt:   time.Now(),
		Metadata:     make(map[string]interface{}),
		Aliases:      []string{},
		Categories:   []string{},
		SubEntities:  []Entity{},
		IsAtomic:     true,
	}
}

// generateEntityID генерирует детерминированный уникальный ID сущности
func generateEntityID(text string, start, end int, entityType string) string {
	input := fmt.Sprintf("%s:%d:%d:%s", text, start, end, entityType)
	hash := md5.Sum([]byte(input))
	return hex.EncodeToString(hash[:])[:16] // короткий, но уникальный hex
}

// AddCategory добавляет категорию, если её ещё нет
func (e *Entity) AddCategory(category string) {
	for _, cat := range e.Categories {
		if cat == category {
			return
		}
	}
	e.Categories = append(e.Categories, category)
}

// AddAlias добавляет алиас, если его ещё нет
func (e *Entity) AddAlias(alias string) {
	for _, a := range e.Aliases {
		if a == alias {
			return
		}
	}
	e.Aliases = append(e.Aliases, alias)
}

// AddMetadata добавляет произвольные метаданные
func (e *Entity) AddMetadata(key string, value interface{}) {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
}

// SetContext sets the context around the entity (byte‑based indices)
func (e *Entity) SetContext(fullText string, contextSize int) {
    // Защита от некорректных позиций
    if e.Start < 0 {
        e.Start = 0
    }
    if e.End > len(fullText) {
        e.End = len(fullText)
    }
    if e.Start > e.End {
        e.Start = e.End
    }

    // Вычисляем границы контекста в байтах
    start := max(0, e.Start-contextSize)
    end := min(len(fullText), e.End+contextSize)

    var sb strings.Builder
    if start < e.Start {
        sb.WriteString("...")
        sb.WriteString(fullText[start:e.Start])
    } else {
        sb.WriteString(fullText[start:e.Start])
    }
    sb.WriteString(fullText[e.Start:e.End])
    if end > e.End {
        sb.WriteString(fullText[e.End:end])
        sb.WriteString("...")
    }
    e.Context = sb.String()
}

// Overlaps проверяет пересечение с другой сущностью
func (e *Entity) Overlaps(other *Entity) bool {
	return !(e.End <= other.Start || e.Start >= other.End)
}

// Contains проверяет, содержит ли эта сущность другую
func (e *Entity) Contains(other *Entity) bool {
	return e.Start <= other.Start && e.End >= other.End
}

// DistanceTo возвращает расстояние между сущностями (в символах)
func (e *Entity) DistanceTo(other *Entity) int {
	if e.Overlaps(other) {
		return 0
	}
	if e.End <= other.Start {
		return other.Start - e.End
	}
	return e.Start - other.End
}

// Helper functions (min/max) — если их нет в пакете, можно добавить здесь или в helpers.go
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}