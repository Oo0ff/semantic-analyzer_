package registry

import (
	"encoding/json"
	"fmt"
	"os"
)

// Term — структура из termRegistry.json (по схеме задачи)
type Term struct {
	UUID       string            `json:"uuid"`
	Definition string            `json:"definition"`
	Variants   []string          `json:"variants"`
	Patterns   []string          `json:"patterns"`
	Class      string            `json:"class"`
	Domain     string            `json:"domain"`
	Codes      map[string]string `json:"codes"`
	// Можно добавить Attributes, Examples, Deprecated и т.д.
}

// LoadTerms — загрузка termRegistry.json
func LoadTerms(path string) ([]Term, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read term registry: %w", err)
	}

	var reg struct {
		SchemaVersion string `json:"schemaVersion"`
		Version       string `json:"version"`
		Terms         []Term `json:"terms"`
	}

	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("failed to parse term registry: %w", err)
	}

	if len(reg.Terms) == 0 {
		return nil, fmt.Errorf("no terms found in %s", path)
	}

	fmt.Printf("Loaded %d terms from %s\n", len(reg.Terms), path)
	return reg.Terms, nil
}