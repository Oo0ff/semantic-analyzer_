package domain

import (
    "encoding/json"
    "fmt"
    "time"
)

// AuditEvent represents a step in the decision chain
type AuditEvent struct {
    ID            string                 `json:"id"`
    Timestamp     time.Time              `json:"timestamp"`
    Stage         string                 `json:"stage"`
    Action        string                 `json:"action"`
    DataSnapshot  map[string]interface{} `json:"data_snapshot"`
    ParentID      string                 `json:"parent_id,omitempty"`
    Children      []string               `json:"children,omitempty"`
    MarkerID      string                 `json:"marker_id,omitempty"`
    RuleID        string                 `json:"rule_id,omitempty"`
    Confidence    float64                `json:"confidence,omitempty"`
    Duration      time.Duration          `json:"duration_ns,omitempty"`
    Error         string                 `json:"error,omitempty"`
    Level         int                    `json:"level,omitempty"`
    Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// NewAuditEvent creates a new audit event
func NewAuditEvent(stage, action string) *AuditEvent {
    now := time.Now()
    return &AuditEvent{
        ID:           fmt.Sprintf("audit_%d", now.UnixNano()),
        Timestamp:    now,
        Stage:        stage,
        Action:       action,
        DataSnapshot: make(map[string]interface{}),
        Children:     []string{},
        Metadata:     make(map[string]interface{}),
    }
}

// AddData adds data to the snapshot
func (ae *AuditEvent) AddData(key string, value interface{}) {
    if ae.DataSnapshot == nil {
        ae.DataSnapshot = make(map[string]interface{})
    }
    ae.DataSnapshot[key] = value
}

// GetData retrieves data from snapshot
func (ae *AuditEvent) GetData(key string) (interface{}, bool) {
    if ae.DataSnapshot == nil {
        return nil, false
    }
    value, exists := ae.DataSnapshot[key]
    return value, exists
}

// AddChild adds a child event ID
func (ae *AuditEvent) AddChild(childID string) {
    if ae.Children == nil {
        ae.Children = []string{}
    }
    ae.Children = append(ae.Children, childID)
}

// SetDuration sets the duration of the event
func (ae *AuditEvent) SetDuration(start time.Time) {
    ae.Duration = time.Since(start)
}

// AddMetadata adds metadata to the event
func (ae *AuditEvent) AddMetadata(key string, value interface{}) {
    if ae.Metadata == nil {
        ae.Metadata = make(map[string]interface{})
    }
    ae.Metadata[key] = value
}

// GetMetadata retrieves metadata by key
func (ae *AuditEvent) GetMetadata(key string) (interface{}, bool) {
    if ae.Metadata == nil {
        return nil, false
    }
    value, exists := ae.Metadata[key]
    return value, exists
}

// ToJSON serializes the audit event to JSON
func (ae *AuditEvent) ToJSON() ([]byte, error) {
    return json.MarshalIndent(ae, "", "  ")
}

// FromJSON deserializes JSON into an audit event
func (ae *AuditEvent) FromJSON(data []byte) error {
    return json.Unmarshal(data, ae)
}

// Validate checks if the audit event is valid
func (ae *AuditEvent) Validate() error {
    if ae.Stage == "" {
        return &AuditValidationError{Field: "Stage", Reason: "cannot be empty"}
    }
    
    if ae.Action == "" {
        return &AuditValidationError{Field: "Action", Reason: "cannot be empty"}
    }
    
    if ae.Timestamp.IsZero() {
        return &AuditValidationError{Field: "Timestamp", Reason: "must be set"}
    }
    
    return nil
}

// AuditTrail represents a collection of audit events with tree structure
type AuditTrail struct {
    RootEventID string                 `json:"root_event_id"`
    Events      map[string]*AuditEvent `json:"events"`
    StartTime   time.Time              `json:"start_time"`
    EndTime     time.Time              `json:"end_time,omitempty"`
    TranscriptID string                `json:"transcript_id,omitempty"`
}

// NewAuditTrail creates a new audit trail
func NewAuditTrail(rootEvent *AuditEvent) *AuditTrail {
    return &AuditTrail{
        RootEventID: rootEvent.ID,
        Events:      map[string]*AuditEvent{rootEvent.ID: rootEvent},
        StartTime:   rootEvent.Timestamp,
    }
}

// AddEvent adds an event to the audit trail
func (at *AuditTrail) AddEvent(event *AuditEvent) error {
    if event == nil {
        return fmt.Errorf("cannot add nil event")
    }
    
    if at.Events == nil {
        at.Events = make(map[string]*AuditEvent)
    }
    
    // Check if event already exists
    if _, exists := at.Events[event.ID]; exists {
        return fmt.Errorf("event with ID %s already exists", event.ID)
    }
    
    at.Events[event.ID] = event
    
    // Update end time
    if at.EndTime.IsZero() || event.Timestamp.After(at.EndTime) {
        at.EndTime = event.Timestamp
    }
    
    return nil
}

// GetEvent retrieves an event by ID
func (at *AuditTrail) GetEvent(id string) (*AuditEvent, bool) {
    event, exists := at.Events[id]
    return event, exists
}

// GetEventsByStage retrieves all events for a specific stage
func (at *AuditTrail) GetEventsByStage(stage string) []*AuditEvent {
    var events []*AuditEvent
    for _, event := range at.Events {
        if event.Stage == stage {
            events = append(events, event)
        }
    }
    return events
}

// ToJSON serializes the audit trail to JSON
func (at *AuditTrail) ToJSON() ([]byte, error) {
    return json.MarshalIndent(at, "", "  ")
}

// FromJSON deserializes JSON into an audit trail
func (at *AuditTrail) FromJSON(data []byte) error {
    return json.Unmarshal(data, at)
}

// Error type for audit validation
type AuditValidationError struct {
    Field  string
    Reason string
}

func (e *AuditValidationError) Error() string {
    return fmt.Sprintf("audit validation error: %s - %s", e.Field, e.Reason)
}