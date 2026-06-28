package audit

import (
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "semantic-analyzer/internal/domain"
)

// Logger provides hierarchical audit logging
type Logger struct {
    rootEvent   *domain.AuditEvent
    events      map[string]*domain.AuditEvent
    parentMap   map[string]string // childID -> parentID
    childrenMap map[string][]string // parentID -> childIDs
    mu          sync.RWMutex
    enabled     bool
    maxDepth    int
}

// NewLogger creates a new audit logger
func NewLogger(rootStage, rootAction string) *Logger {
    rootEvent := domain.NewAuditEvent(rootStage, rootAction)
    
    return &Logger{
        rootEvent:   rootEvent,
        events:      map[string]*domain.AuditEvent{rootEvent.ID: rootEvent},
        parentMap:   make(map[string]string),
        childrenMap: make(map[string][]string),
        enabled:     true,
        maxDepth:    10,
    }
}

// LogEvent logs a new audit event
func (l *Logger) LogEvent(event *domain.AuditEvent, parentID string) string {
    if !l.enabled {
        return ""
    }
    
    l.mu.Lock()
    defer l.mu.Unlock()
    
    // Ensure event has an ID
    if event.ID == "" {
        event.ID = fmt.Sprintf("audit_%d", time.Now().UnixNano())
    }
    
    // Add to events map
    l.events[event.ID] = event
    
    // Set parent-child relationship
    if parentID != "" {
        // Validate parent exists
        if _, exists := l.events[parentID]; exists {
            l.parentMap[event.ID] = parentID
            l.childrenMap[parentID] = append(l.childrenMap[parentID], event.ID)
            
            // Add child to parent event
            parent := l.events[parentID]
            parent.AddChild(event.ID)
        }
    } else {
        // If no parent specified, use root as parent
        l.parentMap[event.ID] = l.rootEvent.ID
        l.childrenMap[l.rootEvent.ID] = append(l.childrenMap[l.rootEvent.ID], event.ID)
        l.rootEvent.AddChild(event.ID)
    }
    
    return event.ID
}

// LogWithParent logs an event with a parent event
func (l *Logger) LogWithParent(stage, action, parentID string) *domain.AuditEvent {
    event := domain.NewAuditEvent(stage, action)
    l.LogEvent(event, parentID)
    return event
}

// LogError logs an error event
func (l *Logger) LogError(parentID, errorMsg string, data map[string]interface{}) string {
    event := domain.NewAuditEvent("error", "occurred")
    event.Error = errorMsg
    
    if data != nil {
        for k, v := range data {
            event.AddData(k, v)
        }
    }
    
    return l.LogEvent(event, parentID)
}

// LogDuration logs the duration of an operation
func (l *Logger) LogDuration(eventID string, startTime time.Time) {
    if !l.enabled {
        return
    }
    
    l.mu.RLock()
    event, exists := l.events[eventID]
    l.mu.RUnlock()
    
    if exists {
        event.SetDuration(startTime)
    }
}

// GetEvent retrieves an event by ID
func (l *Logger) GetEvent(eventID string) (*domain.AuditEvent, bool) {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    event, exists := l.events[eventID]
    return event, exists
}

// GetEventsByStage retrieves all events for a stage
func (l *Logger) GetEventsByStage(stage string) []*domain.AuditEvent {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    var events []*domain.AuditEvent
    for _, event := range l.events {
        if event.Stage == stage {
            events = append(events, event)
        }
    }
    
    return events
}

// GetChildren retrieves child events for a parent
func (l *Logger) GetChildren(parentID string) []*domain.AuditEvent {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    childIDs, exists := l.childrenMap[parentID]
    if !exists {
        return nil
    }
    
    var children []*domain.AuditEvent
    for _, childID := range childIDs {
        if child, exists := l.events[childID]; exists {
            children = append(children, child)
        }
    }
    
    return children
}

// GetParent retrieves the parent of an event
func (l *Logger) GetParent(eventID string) (*domain.AuditEvent, bool) {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    parentID, exists := l.parentMap[eventID]
    if !exists {
        return nil, false
    }
    
    parent, exists := l.events[parentID]
    return parent, exists
}

// GetAncestry retrieves the chain of ancestors for an event
func (l *Logger) GetAncestry(eventID string) []*domain.AuditEvent {
    var ancestry []*domain.AuditEvent
    
    currentID := eventID
    depth := 0
    
    for depth < l.maxDepth {
        parent, exists := l.GetParent(currentID)
        if !exists {
            break
        }
        
        ancestry = append([]*domain.AuditEvent{parent}, ancestry...)
        currentID = parent.ID
        
        if currentID == l.rootEvent.ID {
            break
        }
        
        depth++
    }
    
    return ancestry
}

// GetEventTree retrieves the event tree starting from root
func (l *Logger) GetEventTree() map[string]interface{} {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    return l.buildTree(l.rootEvent.ID, 0)
}

// buildTree recursively builds a tree structure
func (l *Logger) buildTree(eventID string, depth int) map[string]interface{} {
    if depth > l.maxDepth {
        return nil
    }
    
    event, exists := l.events[eventID]
    if !exists {
        return nil
    }
    
    tree := map[string]interface{}{
        "id":        event.ID,
        "stage":     event.Stage,
        "action":    event.Action,
        "timestamp": event.Timestamp,
    }
    
    if event.DataSnapshot != nil {
        tree["data"] = event.DataSnapshot
    }
    
    if event.Error != "" {
        tree["error"] = event.Error
    }
    
    if event.Duration > 0 {
        tree["duration"] = event.Duration
    }
    
    // Add children recursively
    childIDs := l.childrenMap[eventID]
    if len(childIDs) > 0 {
        children := make([]map[string]interface{}, 0, len(childIDs))
        for _, childID := range childIDs {
            childTree := l.buildTree(childID, depth+1)
            if childTree != nil {
                children = append(children, childTree)
            }
        }
        if len(children) > 0 {
            tree["children"] = children
        }
    }
    
    return tree
}

// ToAuditTrail converts the logger to an AuditTrail
func (l *Logger) ToAuditTrail() *domain.AuditTrail {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    events := make(map[string]*domain.AuditEvent, len(l.events))
    for id, event := range l.events {
        events[id] = event
    }
    
    return &domain.AuditTrail{
        RootEventID: l.rootEvent.ID,
        Events:      events,
        StartTime:   l.rootEvent.Timestamp,
        EndTime:     l.getLatestTimestamp(),
    }
}

// getLatestTimestamp gets the latest timestamp from all events
func (l *Logger) getLatestTimestamp() time.Time {
    var latest time.Time
    for _, event := range l.events {
        if event.Timestamp.After(latest) {
            latest = event.Timestamp
        }
    }
    return latest
}

// ToJSON serializes the audit log to JSON
func (l *Logger) ToJSON(pretty bool) ([]byte, error) {
    tree := l.GetEventTree()
    
    if pretty {
        return json.MarshalIndent(tree, "", "  ")
    }
    return json.Marshal(tree)
}

// GetStatistics gets statistics about the audit log
func (l *Logger) GetStatistics() map[string]interface{} {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    stats := map[string]interface{}{
        "total_events": len(l.events),
        "root_event_id": l.rootEvent.ID,
        "start_time":   l.rootEvent.Timestamp,
        "end_time":     l.getLatestTimestamp(),
    }
    
    // Count events by stage
    stageCounts := make(map[string]int)
    for _, event := range l.events {
        stageCounts[event.Stage]++
    }
    stats["events_by_stage"] = stageCounts
    
    // Count errors
    errorCount := 0
    for _, event := range l.events {
        if event.Error != "" {
            errorCount++
        }
    }
    stats["error_count"] = errorCount
    
    // Calculate average depth
    totalDepth := 0
    count := 0
    for eventID := range l.events {
        depth := len(l.GetAncestry(eventID))
        totalDepth += depth
        count++
    }
    
    if count > 0 {
        stats["average_depth"] = float64(totalDepth) / float64(count)
        stats["max_depth"] = l.maxDepth
    }
    
    return stats
}

// Enable enables the logger
func (l *Logger) Enable() {
    l.enabled = true
}

// Disable disables the logger
func (l *Logger) Disable() {
    l.enabled = false
}

// SetMaxDepth sets the maximum recursion depth
func (l *Logger) SetMaxDepth(depth int) {
    l.maxDepth = depth
}

// Clear clears all events except root
func (l *Logger) Clear() {
    l.mu.Lock()
    defer l.mu.Unlock()
    
    l.events = map[string]*domain.AuditEvent{l.rootEvent.ID: l.rootEvent}
    l.parentMap = make(map[string]string)
    l.childrenMap = make(map[string][]string)
}