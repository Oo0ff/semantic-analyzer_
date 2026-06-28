package calendar

import (
	"fmt"
	"strings"
	"time"
)

// ExportToICS преобразует список событий в строку в формате iCalendar (ICS)
func ExportToICS(events []Event) string {
	var sb strings.Builder
	sb.WriteString("BEGIN:VCALENDAR\r\n")
	sb.WriteString("VERSION:2.0\r\n")
	sb.WriteString("PRODID:-//SemanticAnalyzer//Calendar//RU\r\n")

	for _, ev := range events {
		sb.WriteString("BEGIN:VEVENT\r\n")
		sb.WriteString(fmt.Sprintf("DTSTART:%s\r\n", formatICSDateTime(ev.Start, ev.AllDay)))
		if !ev.End.IsZero() {
			sb.WriteString(fmt.Sprintf("DTEND:%s\r\n", formatICSDateTime(ev.End, ev.AllDay)))
		}
		sb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(ev.Title)))
		if ev.Description != "" {
			sb.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICS(ev.Description)))
		}
		if ev.Location != "" {
			sb.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICS(ev.Location)))
		}
		sb.WriteString("END:VEVENT\r\n")
	}

	sb.WriteString("END:VCALENDAR\r\n")
	return sb.String()
}

func formatICSDateTime(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("20060102")
	}
	return t.Format("20060102T150405")
}

func escapeICS(s string) string {
	// простейшее экранирование (запятые, точки с запятой, переносы строк)
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}