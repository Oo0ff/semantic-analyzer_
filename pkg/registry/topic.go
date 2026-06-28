package registry

import (
	"encoding/json"
	"fmt"
	"os"
)

// Topic — структура из topicRegistry.json
type Topic struct {
	TopicCode  string   `json:"topicCode"`
	Label      string   `json:"label"`
	Definition string   `json:"definition"`
	Patterns   []string `json:"patterns"`
	Priority   int      `json:"priority"`
	// Можно добавить Attributes и т.д.
}

// LoadTopics — загрузка topicRegistry.json
func LoadTopics(path string) ([]Topic, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read topic registry: %w", err)
	}

	var reg struct {
		SchemaVersion string  `json:"schemaVersion"`
		Version       string  `json:"version"`
		Topics        []Topic `json:"topics"`
	}

	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("failed to parse topic registry: %w", err)
	}

	if len(reg.Topics) == 0 {
		return nil, fmt.Errorf("no topics found in %s", path)
	}

	fmt.Printf("Loaded %d topics from %s\n", len(reg.Topics), path)
	return reg.Topics, nil
}