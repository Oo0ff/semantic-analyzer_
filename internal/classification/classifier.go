package classification

import (
	"strings"

	"semantic-analyzer/internal/domain"
)

// KeywordWeight – ключевое слово с опциональным весом
type KeywordWeight struct {
	Word   string  `json:"word" yaml:"word" mapstructure:"word"`
	Weight float64 `json:"weight" yaml:"weight" mapstructure:"weight"`
}

// TopicConfig описывает тему со списком ключевых слов (теперь с весами)
type TopicConfig struct {
	Code     string          `mapstructure:"code" yaml:"code"`
	Label    string          `mapstructure:"label" yaml:"label"`
	Keywords []KeywordWeight `mapstructure:"keywords" yaml:"keywords"`
}

// Config – конфигурация классификатора
type Config struct {
	Topics        []TopicConfig `mapstructure:"topics" yaml:"topics"`
	MinConfidence float64       `mapstructure:"min_confidence" yaml:"min_confidence"`
}

// Result – результат классификации
type Result struct {
	Code       string  `json:"code"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
}

// Classifier – rule‑based тематический классификатор
type Classifier struct {
	topics []TopicConfig
}

// New создаёт новый Classifier
func New(cfg Config) *Classifier {
	return &Classifier{topics: cfg.Topics}
}

// Classify анализирует переданный текст и возвращает список подходящих тем
// с учётом весов ключевых слов.
func (c *Classifier) Classify(text string, markers []domain.Marker) []Result {
	if len(c.topics) == 0 {
		return nil
	}

	lowerText := strings.ToLower(text)

	var results []Result
	for _, topic := range c.topics {
		var totalWeight float64
		var matchedWeight float64

		for _, kw := range topic.Keywords {
			// Если вес не указан (0), считаем его равным 1.0
			w := kw.Weight
			if w == 0 {
				w = 1.0
			}
			totalWeight += w

			if strings.Contains(lowerText, strings.ToLower(kw.Word)) {
				matchedWeight += w
			}
		}

		if totalWeight > 0 {
			conf := matchedWeight / totalWeight
			if conf > 0 {
				results = append(results, Result{
					Code:       topic.Code,
					Label:      topic.Label,
					Confidence: conf,
				})
			}
		}
	}
	return results
}