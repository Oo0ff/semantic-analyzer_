package final

import (
    "encoding/json"
    "fmt"
    "strings"
    "text/template"
    "time"

    "semantic-analyzer/internal/domain"
    "gopkg.in/yaml.v3"
)

// OutputFormatter handles formatting of analysis results
type OutputFormatter struct {
    config      OutputConfig
    templates   map[string]*template.Template
    auditEvents []domain.AuditEvent
}

// NewOutputFormatter creates a new output formatter
func NewOutputFormatter(cfg OutputConfig) *OutputFormatter {
    of := &OutputFormatter{
        config:      cfg,
        templates:   make(map[string]*template.Template),
        auditEvents: []domain.AuditEvent{},
    }
    
    of.initialize()
    return of
}

// initialize sets up the output formatter
func (of *OutputFormatter) initialize() {
    // Initialize templates if using text format
    if of.config.Format == "text" {
        of.initializeTemplates()
    }
}

// initializeTemplates initializes text templates
func (of *OutputFormatter) initializeTemplates() {
    // Default template for markers
    markerTemplate := `{{range .Markers}}
[{{.Level}}] {{.Type}}: "{{.TextSpan}}"
  Confidence: {{printf "%.2f" .Confidence}}
  Position: {{.Start}}-{{.End}}
  Rule: {{.RuleID}}
{{if .Metadata}}  Metadata: {{.Metadata}}
{{end}}{{end}}`
    
    tmpl, err := template.New("markers").Parse(markerTemplate)
    if err == nil {
        of.templates["markers"] = tmpl
    }
    
    // Summary template
    summaryTemplate := `Analysis Results
================
Transcript: {{.Transcript.Source}}
Processed: {{.Timestamp.Format "2006-01-02 15:04:05"}}
Duration: {{printf "%.2f" .Statistics.ProcessingTime}}s

Statistics:
  Total Markers: {{.Statistics.TotalMarkers}}
  Atomic Markers: {{.Statistics.AtomicMarkers}}
  Composite Markers: {{.Statistics.CompositeMarkers}}
  Average Confidence: {{printf "%.2f" .Statistics.AverageConfidence}}

Marker Types:{{range $type, $count := .Statistics.MarkerTypes}}
  {{$type}}: {{$count}}{{end}}`
    
    tmpl, err = template.New("summary").Parse(summaryTemplate)
    if err == nil {
        of.templates["summary"] = tmpl
    }
}

// FormatResult formats the analysis result according to configuration
func (of *OutputFormatter) FormatResult(result *domain.AnalysisResult) ([]byte, error) {
    startTime := time.Now()
    
    auditEvent := domain.NewAuditEvent("output_formatter", "format_start")
    auditEvent.AddData("format", of.config.Format)
    auditEvent.AddData("pretty_print", of.config.PrettyPrint)
    of.addAuditEvent(auditEvent)
    
    var formatted []byte
    var err error
    
    switch strings.ToLower(of.config.Format) {
    case "json":
        formatted, err = of.formatJSON(result)
    case "yaml":
        formatted, err = of.formatYAML(result)
    case "text":
        formatted, err = of.formatText(result)
    default:
        err = fmt.Errorf("unsupported format: %s", of.config.Format)
    }
    
    if err != nil {
        errorEvent := domain.NewAuditEvent("output_formatter", "format_error")
        errorEvent.AddData("error", err.Error())
        of.addAuditEvent(errorEvent)
        return nil, err
    }
    
    endEvent := domain.NewAuditEvent("output_formatter", "format_complete")
    endEvent.AddData("output_size", len(formatted))
    endEvent.SetDuration(startTime)
    of.addAuditEvent(endEvent)
    
    return formatted, nil
}

// formatJSON formats result as JSON
func (of *OutputFormatter) formatJSON(result *domain.AnalysisResult) ([]byte, error) {
    // Prepare data for JSON output
    outputData := of.prepareOutputData(result)
    
    if of.config.PrettyPrint {
        return json.MarshalIndent(outputData, "", "  ")
    }
    return json.Marshal(outputData)
}

// formatYAML formats result as YAML
func (of *OutputFormatter) formatYAML(result *domain.AnalysisResult) ([]byte, error) {
    outputData := of.prepareOutputData(result)
    return yaml.Marshal(outputData)
}

// formatText formats result as human-readable text
func (of *OutputFormatter) formatText(result *domain.AnalysisResult) ([]byte, error) {
    var output strings.Builder
    
    // Add summary
    if summaryTmpl, exists := of.templates["summary"]; exists {
        if err := summaryTmpl.Execute(&output, result); err != nil {
            return nil, fmt.Errorf("failed to execute summary template: %w", err)
        }
        output.WriteString("\n\n")
    }
    
    // Add markers by level
    for level := 1; level <= 5; level++ {
        levelMarkers := result.GetMarkersByLevel(level)
        if len(levelMarkers) > 0 {
            output.WriteString(fmt.Sprintf("\n=== Level %d Markers ===\n", level))
            
            if markerTmpl, exists := of.templates["markers"]; exists {
                data := struct{ Markers []domain.Marker }{Markers: levelMarkers}
                if err := markerTmpl.Execute(&output, data); err != nil {
                    return nil, fmt.Errorf("failed to execute marker template: %w", err)
                }
            } else {
                // Fallback to simple format
                for _, marker := range levelMarkers {
                    output.WriteString(fmt.Sprintf("[%d] %s: \"%s\" (%.2f)\n", 
                        marker.Level, marker.Type, marker.TextSpan, marker.Confidence))
                }
            }
        }
    }
    
    // Add audit trail if requested
    if of.config.IncludeAudit && result.AuditTrail != nil {
        output.WriteString("\n=== Audit Trail ===\n")
        output.WriteString(fmt.Sprintf("Events: %d\n", len(result.AuditTrail.Events)))
        output.WriteString(fmt.Sprintf("Duration: %v\n", result.AuditTrail.EndTime.Sub(result.AuditTrail.StartTime)))
    }
    
    return []byte(output.String()), nil
}

// prepareOutputData prepares data for output based on configuration
func (of *OutputFormatter) prepareOutputData(result *domain.AnalysisResult) map[string]interface{} {
    data := make(map[string]interface{})
    
    // Always include basic information
    data["transcript_id"] = result.TranscriptID
    data["timestamp"] = result.Timestamp.Format(of.getTimestampFormat())
    data["version"] = result.Version
    
    // Include statistics
    data["statistics"] = result.Statistics
    
    // Include markers (filtered by requested fields)
    markersData := of.prepareMarkersData(result.Markers)
    data["markers"] = markersData
    
    // Include transcript info if requested
    if of.config.IncludeMetadata && result.Transcript != nil {
        data["transcript"] = map[string]interface{}{
            "source":    result.Transcript.Source,
            "word_count": result.Transcript.TokenCount,
            "metadata":  result.Transcript.Metadata,
        }
    }
    
    // Include audit trail if requested
    if of.config.IncludeAudit && result.AuditTrail != nil {
        data["audit_trail"] = of.prepareAuditData(result.AuditTrail)
    }
    
    // Include processing info
    data["processing"] = map[string]interface{}{
        "config_hash": result.ConfigHash,
        "time_seconds": result.Statistics.ProcessingTime,
    }
    
    return data
}

// prepareMarkersData prepares marker data for output
func (of *OutputFormatter) prepareMarkersData(markers []domain.Marker) []map[string]interface{} {
    var markersData []map[string]interface{}
    
    for _, marker := range markers {
        markerData := make(map[string]interface{})
        
        // If specific fields are requested, only include those
        if len(of.config.Fields) > 0 {
            for _, field := range of.config.Fields {
                value := of.getMarkerField(&marker, field)
                // Apply field mapping if specified
                outputField := field
                if mapped, exists := of.config.FieldMappings[field]; exists {
                    outputField = mapped
                }
                markerData[outputField] = value
            }
        } else {
            // Include all fields
            markerData["id"] = marker.ID
            markerData["level"] = marker.Level
            markerData["type"] = marker.Type
            markerData["text"] = marker.TextSpan
            markerData["start"] = marker.Start
            markerData["end"] = marker.End
            markerData["confidence"] = marker.Confidence
            markerData["rule_id"] = marker.RuleID
            markerData["is_atomic"] = marker.IsAtomic
            
            // Include context if available
            if marker.Context != "" {
                markerData["context"] = marker.Context
            }
            
            // Include metadata if requested
            if of.config.IncludeMetadata && marker.Metadata != nil {
                markerData["metadata"] = marker.Metadata
            }
        }
        
        markersData = append(markersData, markerData)
    }
    
    return markersData
}

// getMarkerField gets a specific field from a marker
func (of *OutputFormatter) getMarkerField(marker *domain.Marker, field string) interface{} {
    switch field {
    case "id":
        return marker.ID
    case "level":
        return marker.Level
    case "type":
        return marker.Type
    case "text", "text_span":
        return marker.TextSpan
    case "start":
        return marker.Start
    case "end":
        return marker.End
    case "confidence":
        return marker.Confidence
    case "rule_id":
        return marker.RuleID
    case "context":
        return marker.Context
    case "is_atomic":
        return marker.IsAtomic
    case "score":
        return marker.Score
    case "weight":
        return marker.Weight
    case "sentence_id":
        return marker.SentenceID
    default:
        // Check metadata
        if of.config.IncludeMetadata && marker.Metadata != nil {
            if value, exists := marker.Metadata[field]; exists {
                return value
            }
        }
        return nil
    }
}

// prepareAuditData prepares audit data for output
func (of *OutputFormatter) prepareAuditData(auditTrail *domain.AuditTrail) map[string]interface{} {
    eventsData := make([]map[string]interface{}, 0, len(auditTrail.Events))
    
    for _, event := range auditTrail.Events {
        eventData := map[string]interface{}{
            "id":        event.ID,
            "timestamp": event.Timestamp.Format(of.getTimestampFormat()),
            "stage":     event.Stage,
            "action":    event.Action,
        }
        
        if event.DataSnapshot != nil {
            eventData["data"] = event.DataSnapshot
        }
        
        if event.RuleID != "" {
            eventData["rule_id"] = event.RuleID
        }
        
        if event.MarkerID != "" {
            eventData["marker_id"] = event.MarkerID
        }
        
        if event.Confidence > 0 {
            eventData["confidence"] = event.Confidence
        }
        
        if event.Duration > 0 {
            eventData["duration_ns"] = event.Duration.Nanoseconds()
        }
        
        if event.Error != "" {
            eventData["error"] = event.Error
        }
        
        eventsData = append(eventsData, eventData)
    }
    
    return map[string]interface{}{
        "root_event_id": auditTrail.RootEventID,
        "start_time":    auditTrail.StartTime.Format(of.getTimestampFormat()),
        "end_time":      auditTrail.EndTime.Format(of.getTimestampFormat()),
        "event_count":   len(auditTrail.Events),
        "events":        eventsData,
    }
}

// getTimestampFormat returns the timestamp format to use
func (of *OutputFormatter) getTimestampFormat() string {
    if of.config.TimestampFormat != "" {
        return of.config.TimestampFormat
    }
    return time.RFC3339
}

// addAuditEvent adds an audit event
func (of *OutputFormatter) addAuditEvent(event *domain.AuditEvent) {
    of.auditEvents = append(of.auditEvents, *event)
}

// GetAuditEvents returns all audit events
func (of *OutputFormatter) GetAuditEvents() []domain.AuditEvent {
    return of.auditEvents
}