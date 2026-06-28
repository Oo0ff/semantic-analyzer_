package summarization

import (
	"sort"
	"strings"

	"semantic-analyzer/internal/domain"
)

// Config параметры экстрактивной суммаризации
type Config struct {
	MaxSentences    int     `mapstructure:"max_sentences" yaml:"max_sentences"`
	PositionWeight  float64 `mapstructure:"position_weight" yaml:"position_weight"`
	DiversityWeight float64 `mapstructure:"diversity_weight" yaml:"diversity_weight"` // 0 – без MMR, >0 – вес разнообразия (0..1)
}

// Summarizer выполняет экстрактивную суммаризацию на основе важности предложений
type Summarizer struct {
	config Config
}

// New создаёт новый Summarizer
func New(cfg Config) *Summarizer {
	if cfg.MaxSentences <= 0 {
		cfg.MaxSentences = 3
	}
	// По умолчанию diversity_weight = 0.3, если не задан
	if cfg.DiversityWeight == 0 {
		cfg.DiversityWeight = 0.3
	}
	return &Summarizer{config: cfg}
}

// sentenceImportance – вспомогательная структура для ранжирования
type sentenceImportance struct {
	index      int
	text       string
	importance float64
	words      []string // множество слов для вычисления сходства
}

// wordsSet возвращает множество уникальных слов предложения (в нижнем регистре)
func wordsSet(text string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		// простая очистка от знаков препинания
		w = strings.Trim(w, ".,;:!?\"'()[]{}«»")
		if len(w) > 1 {
			set[w] = true
		}
	}
	return set
}

// jaccardSimilarity вычисляет коэффициент Жаккара между двумя множествами слов
func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	intersection := 0
	for w := range a {
		if b[w] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	return float64(intersection) / float64(union)
}

// Summarize возвращает краткое содержание (top N предложений) с опциональным MMR
func (s *Summarizer) Summarize(sentences []string, markers []domain.Marker) string {
	if len(sentences) == 0 {
		return ""
	}

	// Вычисляем важность каждого предложения
	imp := make([]sentenceImportance, len(sentences))
	for i, sent := range sentences {
		lowerSent := strings.ToLower(sent)
		var score float64
		for _, m := range markers {
			if strings.Contains(lowerSent, strings.ToLower(m.TextSpan)) {
				score += domain.FromFixedPoint(m.Confidence)
			}
		}
		positionFactor := 1.0 / (1.0 + s.config.PositionWeight*float64(i))
		imp[i] = sentenceImportance{
			index:      i,
			text:       sent,
			importance: score * positionFactor,
			words:      nil, // заполним позже при необходимости
		}
	}

	// Если diversity_weight = 0, используем старый top-N без MMR
	if s.config.DiversityWeight == 0 {
		sort.Slice(imp, func(i, j int) bool {
			return imp[i].importance > imp[j].importance
		})

		n := s.config.MaxSentences
		if n > len(imp) {
			n = len(imp)
		}
		top := imp[:n]
		sort.Slice(top, func(i, j int) bool {
			return top[i].index < top[j].index
		})

		var sb strings.Builder
		for i, si := range top {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(si.text)
		}
		return sb.String()
	}

	// MMR‑режим: подготовка множеств слов для каждого предложения
	for i := range imp {
		imp[i].words = nil // будет вычислено лениво? Лучше сразу
		imp[i].words = make([]string, 0)
		set := wordsSet(imp[i].text)
		for w := range set {
			imp[i].words = append(imp[i].words, w)
		}
	}

	selectedIndexes := make([]int, 0, s.config.MaxSentences)
	candidateSet := make(map[int]*sentenceImportance)
	for i := range imp {
		candidateSet[i] = &imp[i]
	}

	lambda := s.config.DiversityWeight
	for len(selectedIndexes) < s.config.MaxSentences && len(candidateSet) > 0 {
		bestIdx := -1
		bestScore := -1.0

		for idx, cand := range candidateSet {
			// Вычисляем максимальное сходство с уже выбранными
			maxSim := 0.0
			for _, selIdx := range selectedIndexes {
				sel := &imp[selIdx]
				sim := jaccardSimilarity(wordsSet(cand.text), wordsSet(sel.text))
				if sim > maxSim {
					maxSim = sim
				}
			}

			mmr := (1.0-lambda)*cand.importance - lambda*maxSim
			if mmr > bestScore {
				bestScore = mmr
				bestIdx = idx
			}
		}

		if bestIdx == -1 {
			break
		}

		selectedIndexes = append(selectedIndexes, bestIdx)
		delete(candidateSet, bestIdx)
	}

	// Сортируем отобранные по исходному индексу
	sort.Ints(selectedIndexes)

	var sb strings.Builder
	for i, idx := range selectedIndexes {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(imp[idx].text)
	}
	return sb.String()
}