package semantic

import (
    "fmt"
    "regexp"
    "strconv"
    "strings"
    "time"

    "semantic-analyzer/internal/domain"
    "semantic-analyzer/pkg/config"
)

// LogicalEvaluator evaluates logical expressions against context
type LogicalEvaluator struct {
    config      config.SemanticRulesConfig
    rules       []config.LogicalRuleConfig
    auditEvents []domain.AuditEvent
    cache       map[string]bool // Expression caching for performance
    traceID     string          // provenance
}

// NewLogicalEvaluator creates a new logical evaluator with trace ID
func NewLogicalEvaluator(cfg config.SemanticRulesConfig, traceID string) *LogicalEvaluator {
    return &LogicalEvaluator{
        config:      cfg,
        rules:       cfg.LogicalRules,
        auditEvents: []domain.AuditEvent{},
        cache:       make(map[string]bool),
        traceID:     traceID,
    }
}

// EvaluateExpression evaluates a logical expression against context
func (le *LogicalEvaluator) EvaluateExpression(expr string, context map[string]interface{}) bool {
    startTime := time.Now()
    
    // Check cache first
    cacheKey := le.generateCacheKey(expr, context)
    if result, exists := le.cache[cacheKey]; exists {
        return result
    }
    
    // Create audit event with trace ID
    auditEvent := domain.NewAuditEvent("logical_evaluator", "evaluate_expression")
    auditEvent.AddData("expression", expr)
    auditEvent.AddData("trace_id", le.traceID)
    auditEvent.AddData("weight", 0.85) // example weight
    le.addAuditEvent(auditEvent)
    
    // Parse and evaluate expression
    result := le.evaluate(expr, context)
    
    // Cache result
    le.cache[cacheKey] = result
    
    // Create completion audit event
    endEvent := domain.NewAuditEvent("logical_evaluator", "expression_evaluated")
    endEvent.AddData("expression", expr)
    endEvent.AddData("result", result)
    endEvent.AddData("trace_id", le.traceID)
    endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
    endEvent.SetDuration(startTime)
    le.addAuditEvent(endEvent)
    
    return result
}

// EvaluateAllRules evaluates all logical rules against context
func (le *LogicalEvaluator) EvaluateAllRules(context map[string]interface{}) []RuleResult {
    startTime := time.Now()
    
    auditEvent := domain.NewAuditEvent("logical_evaluator", "evaluate_all_rules")
    auditEvent.AddData("rule_count", len(le.rules))
    auditEvent.AddData("trace_id", le.traceID)
    le.addAuditEvent(auditEvent)
    
    var results []RuleResult
    
    for _, rule := range le.rules {
        result := le.EvaluateExpression(rule.Expression, context)
        
        ruleResult := RuleResult{
            RuleID:     rule.ID,
            Type:       rule.Type,
            Level:      rule.Level,
            Weight:     rule.Weight,
            OutputType: rule.OutputType,
            Result:     result,
            Expression: rule.Expression,
            Explanation: le.generateExplanation(rule.Expression, context, result),
        }
        
        results = append(results, ruleResult)
    }
    
    endEvent := domain.NewAuditEvent("logical_evaluator", "all_rules_evaluated")
    endEvent.AddData("rules_evaluated", len(results))
    endEvent.AddData("rules_triggered", le.countTriggeredRules(results))
    endEvent.AddData("trace_id", le.traceID)
    endEvent.AddData("processing_time_ms", time.Since(startTime).Milliseconds())
    endEvent.SetDuration(startTime)
    le.addAuditEvent(endEvent)
    
    return results
}

// evaluate recursively evaluates a logical expression
func (le *LogicalEvaluator) evaluate(expr string, context map[string]interface{}) bool {
    expr = strings.TrimSpace(expr)
    
    // Handle parentheses
    if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
        depth := 0
        complete := true
        for i, char := range expr {
            if char == '(' {
                depth++
            } else if char == ')' {
                depth--
                if depth == 0 && i != len(expr)-1 {
                    complete = false
                    break
                }
            }
        }
        if complete {
            return le.evaluate(expr[1:len(expr)-1], context)
        }
    }
    
    // Handle AND expressions
    if andIndex := le.findOperator(expr, "AND"); andIndex != -1 {
        left := strings.TrimSpace(expr[:andIndex])
        right := strings.TrimSpace(expr[andIndex+3:])
        return le.evaluate(left, context) && le.evaluate(right, context)
    }
    
    // Handle OR expressions
    if orIndex := le.findOperator(expr, "OR"); orIndex != -1 {
        left := strings.TrimSpace(expr[:orIndex])
        right := strings.TrimSpace(expr[orIndex+2:])
        return le.evaluate(left, context) || le.evaluate(right, context)
    }
    
    // Handle NOT expressions
    if strings.HasPrefix(expr, "NOT ") {
        subExpr := strings.TrimSpace(expr[4:])
        return !le.evaluate(subExpr, context)
    }
    
    // Handle atomic expressions
    return le.evaluateAtomic(expr, context)
}

// findOperator finds an operator in an expression, respecting parentheses
func (le *LogicalEvaluator) findOperator(expr, operator string) int {
    depth := 0
    for i := 0; i < len(expr)-len(operator)+1; i++ {
        if expr[i] == '(' {
            depth++
        } else if expr[i] == ')' {
            depth--
        } else if depth == 0 && strings.HasPrefix(expr[i:], operator) {
            beforeOK := i == 0 || expr[i-1] == ' ' || expr[i-1] == '('
            afterOK := i+len(operator) == len(expr) || 
                      expr[i+len(operator)] == ' ' || 
                      expr[i+len(operator)] == ')'
            if beforeOK && afterOK {
                return i
            }
        }
    }
    return -1
}

// evaluateAtomic evaluates an atomic logical expression
func (le *LogicalEvaluator) evaluateAtomic(expr string, context map[string]interface{}) bool {
    expr = strings.TrimSpace(expr)
    
    // Handle MARKER_TYPE:XXX
    if strings.HasPrefix(expr, "MARKER_TYPE:") {
        markerType := strings.TrimPrefix(expr, "MARKER_TYPE:")
        return le.evaluateMarkerType(markerType, context)
    }
    
    // Handle ENTITY_TYPE:XXX
    if strings.HasPrefix(expr, "ENTITY_TYPE:") {
        entityType := strings.TrimPrefix(expr, "ENTITY_TYPE:")
        return le.evaluateEntityType(entityType, context)
    }
    
    // Handle NEAR(XXX, distance) or NEAR(distance)
    if strings.HasPrefix(expr, "NEAR(") && strings.HasSuffix(expr, ")") {
        inner := expr[5 : len(expr)-1]
        parts := strings.SplitN(inner, ",", 2)
        if len(parts) == 2 {
            target := strings.TrimSpace(parts[0])
            distanceStr := strings.TrimSpace(parts[1])
            distance, err := strconv.Atoi(distanceStr)
            if err == nil {
                return le.evaluateNearWithType(target, distance, context)
            }
        } else if len(parts) == 1 {
            // NEAR(distance) – проверяем, есть ли в контексте хотя бы два любых маркера на расстоянии
            distance, err := strconv.Atoi(strings.TrimSpace(parts[0]))
            if err == nil {
                return le.evaluateNearGlobal(distance, context)
            }
        }
    }
    
    // Handle CONTAINS(XXX)
    if strings.HasPrefix(expr, "CONTAINS(") && strings.HasSuffix(expr, ")") {
        inner := expr[9 : len(expr)-1]
        return le.evaluateContains(inner, context)
    }
    
    // Handle literal true/false
    if expr == "true" || expr == "TRUE" {
        return true
    }
    if expr == "false" || expr == "FALSE" {
        return false
    }
    
    // Default: treat as context key
    if val, exists := context[expr]; exists {
        if boolVal, ok := val.(bool); ok {
            return boolVal
        }
    }
    
    return false
}

// evaluateMarkerType checks if a marker type exists in context
func (le *LogicalEvaluator) evaluateMarkerType(markerType string, context map[string]interface{}) bool {
    if markers, exists := context["markers"]; exists {
        if markerList, ok := markers.([]domain.Marker); ok {
            for _, marker := range markerList {
                if marker.Type == markerType {
                    return true
                }
            }
        }
    }
    
    if candidates, exists := context["composite_candidates"]; exists {
        if candidateList, ok := candidates.([]interface{}); ok {
            for _, candidate := range candidateList {
                if c, ok := candidate.(map[string]interface{}); ok {
                    if cType, exists := c["primary_type"]; exists {
                        if cType == markerType {
                            return true
                        }
                    }
                }
            }
        }
    }
    
    return false
}

// evaluateEntityType checks if an entity type exists in context
func (le *LogicalEvaluator) evaluateEntityType(entityType string, context map[string]interface{}) bool {
    if entities, exists := context["entities"]; exists {
        if entityList, ok := entities.([]Entity); ok {
            for _, entity := range entityList {
                if entity.Type == entityType {
                    return true
                }
            }
        }
    }
    return false
}

// evaluateNearWithType проверяет, есть ли как минимум два маркера или сущности указанного типа на расстоянии <= distance
func (le *LogicalEvaluator) evaluateNearWithType(target string, distance int, context map[string]interface{}) bool {
    var targetType string
    var targetValue string
    
    if strings.HasPrefix(target, "MARKER_TYPE:") {
        targetType = "marker"
        targetValue = strings.TrimPrefix(target, "MARKER_TYPE:")
    } else if strings.HasPrefix(target, "ENTITY_TYPE:") {
        targetType = "entity"
        targetValue = strings.TrimPrefix(target, "ENTITY_TYPE:")
    } else {
        return false
    }
    
    var positions []int
    if targetType == "marker" {
        if markers, exists := context["markers"].([]domain.Marker); exists {
            for _, m := range markers {
                if m.Type == targetValue {
                    positions = append(positions, (m.Start+m.End)/2)
                }
            }
        }
    } else {
        if entities, exists := context["entities"].([]Entity); exists {
            for _, e := range entities {
                if e.Type == targetValue {
                    positions = append(positions, (e.Start+e.End)/2)
                }
            }
        }
    }
    
    // Проверяем, есть ли хотя бы два элемента на расстоянии <= distance
    for i := 0; i < len(positions); i++ {
        for j := i + 1; j < len(positions); j++ {
            if abs(positions[i]-positions[j]) <= distance {
                return true
            }
        }
    }
    return false
}

// evaluateNearGlobal проверяет, есть ли любые два маркера на расстоянии <= distance
func (le *LogicalEvaluator) evaluateNearGlobal(distance int, context map[string]interface{}) bool {
    var positions []int
    if markers, exists := context["markers"].([]domain.Marker); exists {
        for _, m := range markers {
            positions = append(positions, (m.Start+m.End)/2)
        }
    }
    // Также можно добавить entities при необходимости
    if entities, exists := context["entities"].([]Entity); exists {
        for _, e := range entities {
            positions = append(positions, (e.Start+e.End)/2)
        }
    }
    
    for i := 0; i < len(positions); i++ {
        for j := i + 1; j < len(positions); j++ {
            if abs(positions[i]-positions[j]) <= distance {
                return true
            }
        }
    }
    return false
}

// evaluateContains checks if text contains a pattern
func (le *LogicalEvaluator) evaluateContains(pattern string, context map[string]interface{}) bool {
    if text, exists := context["text"]; exists {
        if textStr, ok := text.(string); ok {
            if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
                regexPattern := pattern[1 : len(pattern)-1]
                matched, _ := regexp.MatchString(regexPattern, textStr)
                return matched
            } else {
                return strings.Contains(strings.ToLower(textStr), strings.ToLower(pattern))
            }
        }
    }
    return false
}

// generateExplanation … (без изменений, как в исходном файле)
func (le *LogicalEvaluator) generateExplanation(expr string, context map[string]interface{}, result bool) string {
    var explanation strings.Builder
    
    explanation.WriteString(fmt.Sprintf("Expression: %s\n", expr))
    explanation.WriteString(fmt.Sprintf("Result: %v\n", result))
    
    if result {
        explanation.WriteString("Triggered because:\n")
        if strings.Contains(expr, "AND") {
            parts := strings.Split(expr, "AND")
            for _, part := range parts {
                part = strings.TrimSpace(part)
                if le.evaluateAtomic(part, context) {
                    explanation.WriteString(fmt.Sprintf("  - %s is true\n", part))
                }
            }
        } else if strings.Contains(expr, "OR") {
            parts := strings.Split(expr, "OR")
            for _, part := range parts {
                part = strings.TrimSpace(part)
                if le.evaluateAtomic(part, context) {
                    explanation.WriteString(fmt.Sprintf("  - %s is true\n", part))
                    break
                }
            }
        } else {
            explanation.WriteString(fmt.Sprintf("  - %s is true\n", expr))
        }
    } else {
        explanation.WriteString("Not triggered because:\n")
        if strings.Contains(expr, "AND") {
            parts := strings.Split(expr, "AND")
            for _, part := range parts {
                part = strings.TrimSpace(part)
                if !le.evaluateAtomic(part, context) {
                    explanation.WriteString(fmt.Sprintf("  - %s is false\n", part))
                }
            }
        } else if strings.Contains(expr, "OR") {
            parts := strings.Split(expr, "OR")
            allFalse := true
            for _, part := range parts {
                part = strings.TrimSpace(part)
                if le.evaluateAtomic(part, context) {
                    allFalse = false
                    break
                }
            }
            if allFalse {
                explanation.WriteString("  - All OR conditions are false\n")
            }
        } else {
            explanation.WriteString(fmt.Sprintf("  - %s is false\n", expr))
        }
    }
    
    return explanation.String()
}

// generateCacheKey … (без изменений)
func (le *LogicalEvaluator) generateCacheKey(expr string, context map[string]interface{}) string {
    var key strings.Builder
    key.WriteString(expr)
    if markers, exists := context["markers"]; exists {
        if markerList, ok := markers.([]domain.Marker); ok {
            for _, marker := range markerList {
                key.WriteString(marker.ID)
                key.WriteString(marker.Type)
            }
        }
    }
    if entities, exists := context["entities"]; exists {
        if entityList, ok := entities.([]Entity); ok {
            for _, entity := range entityList {
                key.WriteString(entity.ID)
                key.WriteString(entity.Type)
            }
        }
    }
    return key.String()
}

// countTriggeredRules … (без изменений)
func (le *LogicalEvaluator) countTriggeredRules(results []RuleResult) int {
    count := 0
    for _, result := range results {
        if result.Result {
            count++
        }
    }
    return count
}

// addAuditEvent … (без изменений)
func (le *LogicalEvaluator) addAuditEvent(event *domain.AuditEvent) {
    le.auditEvents = append(le.auditEvents, *event)
}

// GetAuditEvents … (без изменений)
func (le *LogicalEvaluator) GetAuditEvents() []domain.AuditEvent {
    return le.auditEvents
}

// RuleResult … (без изменений)
type RuleResult struct {
    RuleID      string  `json:"rule_id"`
    Type        string  `json:"type"`
    Level       int     `json:"level"`
    Weight      float64 `json:"weight"`
    OutputType  string  `json:"output_type"`
    Result      bool    `json:"result"`
    Expression  string  `json:"expression"`
    Explanation string  `json:"explanation"`
}

// Helper function
func abs(x int) int {
    if x < 0 {
        return -x
    }
    return x
}