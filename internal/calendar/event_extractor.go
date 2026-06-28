package calendar

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"semantic-analyzer/internal/domain"
	"github.com/olebedev/when"
	"github.com/olebedev/when/rules/common"
	"github.com/olebedev/when/rules/ru"
)

type Extractor struct {
	parser *when.Parser
}

var dayNames = map[string]time.Weekday{
	"понедельник": time.Monday, "вторник": time.Tuesday, "среда": time.Wednesday,
	"четверг": time.Thursday, "пятница": time.Friday, "суббота": time.Saturday, "воскресенье": time.Sunday,
}

func NewExtractor() *Extractor {
	w := when.New(nil)
	w.Add(ru.All...)
	w.Add(common.All...)
	return &Extractor{parser: w}
}

// Extract теперь принимает customPersons – список имён в нижнем регистре из CSV.
func (e *Extractor) Extract(text string, markers []domain.Marker, sentences []string, customPersons []string) []Event {
	now := time.Now()
	var rawEvents []rawEvent

	sentenceBounds := buildSentenceBounds(text, sentences)

	e.parseWithWhen(text, now, markers, sentenceBounds, &rawEvents)
	e.parseDayPhrases(text, now, markers, sentenceBounds, &rawEvents)

	deduped := deduplicateByStartAndTitle(rawEvents)

	var events []Event
	for _, raw := range deduped {
		assignee := findResponsibleForEvent(raw.pos, raw.title, raw.sourceText, text, markers, sentenceBounds, customPersons)
		// Пропускаем события без ответственного
		if assignee == "" {
			continue
		}
		if raw.title == "Событие" {
			continue
		}
		events = append(events, Event{
			Title:      raw.title,
			Start:      raw.start,
			End:        raw.end,
			SourceText: raw.sourceText,
			Assignee:   assignee,
		})
	}
	return events
}

type rawEvent struct {
	title      string
	start      time.Time
	end        time.Time
	sourceText string
	pos        int
}

func buildSentenceBounds(text string, sentences []string) [][2]int {
	var bounds [][2]int
	offset := 0
	for _, s := range sentences {
		idx := indexAt(text, s, offset)
		if idx == -1 {
			continue
		}
		end := idx + len(s)
		bounds = append(bounds, [2]int{idx, end})
		offset = end
	}
	return bounds
}

func indexAt(s, substr string, start int) int {
	idx := strings.Index(s[start:], substr)
	if idx == -1 {
		return -1
	}
	return start + idx
}

func findSentenceIndex(bounds [][2]int, pos int) int {
	for i, b := range bounds {
		if pos >= b[0] && pos < b[1] {
			return i
		}
	}
	return -1
}

// findBestActionTitle – ищет заголовок события, при необходимости возвращая предложение целиком.
func findBestActionTitle(markers []domain.Marker, bounds [][2]int, text string, pos int) string {
	sentIdx := findSentenceIndex(bounds, pos)
	if sentIdx != -1 {
		for _, mk := range markers {
			if mk.Type == "ACTION" || mk.Type == "TASK" || mk.Type == "MEETING" {
				if findSentenceIndex(bounds, mk.Start) == sentIdx {
					return mk.TextSpan
				}
			}
		}
	}
	bestDist := 300
	bestTitle := "Событие"
	for _, mk := range markers {
		if mk.Type == "ACTION" || mk.Type == "TASK" || mk.Type == "MEETING" {
			dist := abs(pos - (mk.Start+mk.End)/2)
			if dist < bestDist {
				bestDist = dist
				bestTitle = mk.TextSpan
			}
		}
	}
	if bestTitle == "Событие" {
		for _, b := range bounds {
			if pos >= b[0] && pos < b[1] {
				return text[b[0]:b[1]]
			}
		}
	}
	return bestTitle
}

func (e *Extractor) parseWithWhen(text string, now time.Time, markers []domain.Marker,
	bounds [][2]int, events *[]rawEvent) {
	offset := 0
	remaining := text
	for {
		if len(remaining) == 0 {
			break
		}
		res, err := e.parser.Parse(remaining, now)
		if err != nil || res == nil {
			break
		}
		absPos := offset + res.Index
		startTime := res.Time
		endTime := startTime.Add(1 * time.Hour)
		title := findBestActionTitle(markers, bounds, text, absPos)
		*events = append(*events, rawEvent{
			title:      title,
			start:      startTime,
			end:        endTime,
			sourceText: safeSubstring(text, absPos, utf8.RuneCountInString(res.Text), 30),
			pos:        absPos,
		})
		matchLen := len(res.Text)
		offset += res.Index + matchLen
		remaining = text[offset:]
	}
}

func (e *Extractor) parseDayPhrases(text string, now time.Time, markers []domain.Marker,
	bounds [][2]int, events *[]rawEvent) {
	re := regexp.MustCompile(`(?i)(?:к|до)\s+(` + strings.Join(dayNamesKeys(), "|") + `)(?:\s|$|[.,;!?])`)
	matches := re.FindAllStringSubmatchIndex(text, -1)
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		dayStr := strings.ToLower(text[m[2]:m[3]])
		targetDay, ok := dayNames[dayStr]
		if !ok {
			continue
		}
		daysUntil := (int(targetDay) - int(now.Weekday()) + 7) % 7
		if daysUntil == 0 {
			daysUntil = 7
		}
		date := now.AddDate(0, 0, daysUntil)
		startTime := time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, now.Location())
		endTime := startTime.Add(1 * time.Hour)
		pos := m[0]
		title := findBestActionTitle(markers, bounds, text, pos)
		*events = append(*events, rawEvent{
			title:      title,
			start:      startTime,
			end:        endTime,
			sourceText: safeSubstring(text, pos, m[1]-m[0], 30),
			pos:        pos,
		})
	}
}

func dayNamesKeys() []string {
	keys := make([]string, 0, len(dayNames))
	for k := range dayNames {
		keys = append(keys, k)
	}
	return keys
}

func safeSubstring(s string, idx, length, margin int) string {
	start := idx - margin
	if start < 0 {
		start = 0
	}
	end := idx + length + margin
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}

func deduplicateByStartAndTitle(events []rawEvent) []rawEvent {
	seen := make(map[string]bool)
	var unique []rawEvent
	for _, ev := range events {
		key := ev.start.Format(time.RFC3339) + "|" + ev.title
		if !seen[key] {
			seen[key] = true
			unique = append(unique, ev)
		}
	}
	return unique
}

// findResponsibleForEvent ищет любое из переданных имён (в нижнем регистре) в предложении
// с временной меткой и в соседних (±1). Возвращает имя в том виде, в котором оно найдено.
func findResponsibleForEvent(pos int, title, sourceText, fullText string,
	markers []domain.Marker, bounds [][2]int, customPersons []string) string {
	sentIdx := findSentenceIndex(bounds, pos)
	if sentIdx == -1 {
		return ""
	}
	for delta := -1; delta <= 1; delta++ {
		idx := sentIdx + delta
		if idx >= 0 && idx < len(bounds) {
			sent := fullText[bounds[idx][0]:bounds[idx][1]]
			sentLower := strings.ToLower(sent)
			for _, name := range customPersons {
				if strings.Contains(sentLower, name) {
					// Возвращаем в нижнем регистре (можно привести к оригинальному из CSV, но оставим так)
					return name
				}
			}
		}
	}
	return ""
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}