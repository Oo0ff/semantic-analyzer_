package protocol

import (
	"fmt"
	"strings"
	"time"

	"semantic-analyzer/internal/domain"
	"semantic-analyzer/internal/participants"
)

// Generator генерирует протокол встречи в формате Markdown
type Generator struct {
	profiles []participants.Profile
}

// NewGenerator создаёт новый генератор с профилями
func NewGenerator(profiles []participants.Profile) *Generator {
	return &Generator{profiles: profiles}
}

// Generate принимает результат анализа и возвращает протокол в виде строки (Markdown)
func (g *Generator) Generate(result *domain.AnalysisResult, audioFile string) string {
	var sb strings.Builder

	// Заголовок
	sb.WriteString(fmt.Sprintf("# Протокол совещания\n\n"))
	sb.WriteString(fmt.Sprintf("**Дата:** %s\n\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Источник:** %s\n\n", audioFile))

	// Участники (теперь ищем прямо в тексте, а не в маркерах)
	participantsList := g.extractParticipants(result.Transcript.ProcessedText)
	if len(participantsList) > 0 {
		sb.WriteString("## Участники\n\n")
		for _, p := range participantsList {
			sb.WriteString(fmt.Sprintf("- %s\n", p.Name))
		}
		sb.WriteString("\n")
	}

	// Повестка / ключевые тезисы (саммари)
	if result.Summary != "" {
		sb.WriteString("## Ключевые тезисы\n\n")
		sb.WriteString(result.Summary + "\n\n")
	}

	// Темы обсуждения
	if len(result.Topics) > 0 {
		sb.WriteString("## Темы обсуждения\n\n")
		for _, t := range result.Topics {
			sb.WriteString(fmt.Sprintf("- **%s** (%.0f%%)\n", t.Label, t.Confidence*100))
		}
		sb.WriteString("\n")
	}

	// Принятые решения / важные маркеры (уровень >= 2 или определённых типов)
	sb.WriteString("## Принятые решения и договорённости\n\n")
	hasDecisions := false
	for _, m := range result.Markers {
		if m.Level >= 2 || m.Type == "DECISION" || m.Type == "URGENT_TASK" || m.Type == "CRITICAL_BUG" {
			hasDecisions = true
			sb.WriteString(fmt.Sprintf("- [%s] %s: \"%s\" (уверенность: %.0f%%)\n",
				m.Type, m.TextSpan, m.Context, domain.FromFixedPoint(m.Confidence)*100))
		}
	}
	if !hasDecisions {
		sb.WriteString("_Нет зафиксированных решений_\n")
	}
	sb.WriteString("\n")

	// Задачи и действия
	sb.WriteString("## Задачи и действия\n\n")
	taskMarkers := g.filterMarkersByType(result.Markers, "ACTION", "TASK", "TODO")
	if len(taskMarkers) > 0 {
		for _, m := range taskMarkers {
			responsible := g.findResponsible(m.TextSpan)
			due := g.extractDueDate(m)
			sb.WriteString(fmt.Sprintf("- **%s**", m.TextSpan))
			if responsible != "" {
				sb.WriteString(fmt.Sprintf(" (ответственный: %s)", responsible))
			}
			if due != "" {
				sb.WriteString(fmt.Sprintf(" — срок: %s", due))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("_Нет назначенных задач_\n")
	}
	sb.WriteString("\n")

	// Сгенерировано автоматически
	sb.WriteString("---\n")
	sb.WriteString("*Протокол сгенерирован автоматически системой Semantic Analyzer*\n")

	return sb.String()
}

// extractParticipants ищет в тексте транскрипта имена из профилей
func (g *Generator) extractParticipants(transcriptText string) []participants.Profile {
	var found []participants.Profile
	seen := make(map[string]bool)
	lowerText := strings.ToLower(transcriptText)

	for _, p := range g.profiles {
		if p.ID == "default" {
			continue
		}
		if strings.Contains(lowerText, strings.ToLower(p.Name)) {
			if !seen[p.ID] {
				found = append(found, p)
				seen[p.ID] = true
			}
		}
	}
	if len(found) == 0 {
		if def := g.findDefaultProfile(); def != nil {
			found = append(found, *def)
		}
	}
	return found
}

// findProfileByName ищет профиль по имени (частичное совпадение)
func (g *Generator) findProfileByName(name string) *participants.Profile {
	lower := strings.ToLower(name)
	for i, p := range g.profiles {
		if strings.Contains(lower, strings.ToLower(p.Name)) {
			return &g.profiles[i]
		}
	}
	return nil
}

func (g *Generator) findDefaultProfile() *participants.Profile {
	for i, p := range g.profiles {
		if p.ID == "default" {
			return &g.profiles[i]
		}
	}
	return nil
}

func (g *Generator) filterMarkersByType(markers []domain.Marker, types ...string) []domain.Marker {
	var res []domain.Marker
	for _, m := range markers {
		for _, t := range types {
			if m.Type == t {
				res = append(res, m)
				break
			}
		}
	}
	return res
}

// findResponsible ищет имя ответственного в контексте маркера (простая эвристика)
func (g *Generator) findResponsible(text string) string {
	for _, p := range g.profiles {
		if strings.Contains(strings.ToLower(text), strings.ToLower(p.Name)) {
			return p.Name
		}
	}
	return ""
}

// extractDueDate пытается извлечь дату из контекста маркера
func (g *Generator) extractDueDate(marker domain.Marker) string {
	if strings.Contains(marker.Context, "202") {
		return "см. контекст"
	}
	return ""
}