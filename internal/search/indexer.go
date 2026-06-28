package search

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// Document представляет индексируемый документ
type Document struct {
	ID       string    `json:"id"`
	Text     string    `json:"text"`
	Source   string    `json:"source"`
	Date     time.Time `json:"date"`
	Markers  []string  `json:"markers"`
	Topics   []string  `json:"topics"`
	Summary  string    `json:"summary"`
	Protocol string    `json:"protocol"`
}

// Indexer управляет индексом Bleve
type Indexer struct {
	index bleve.Index
	path  string
}

func NewIndexer(indexPath string) (*Indexer, error) {
	if indexPath == "" {
		indexPath = "./search_index"
	}
	os.MkdirAll(indexPath, 0755)

	var index bleve.Index
	// Проверяем, существует ли валидный индекс
	if _, err := os.Stat(filepath.Join(indexPath, "index_meta.json")); err == nil {
		index, err = bleve.Open(indexPath)
		if err != nil {
			os.RemoveAll(indexPath)
			os.MkdirAll(indexPath, 0755)
		} else {
			return &Indexer{index: index, path: indexPath}, nil
		}
	}

	// Создаём новый индекс
	indexMapping := buildIndexMapping()
	index, err := bleve.New(indexPath, indexMapping)
	if err != nil {
		return nil, fmt.Errorf("cannot create index: %w", err)
	}
	return &Indexer{index: index, path: indexPath}, nil
}

func buildIndexMapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()
	docMapping := bleve.NewDocumentMapping()

	// Поле text – текстовое (анализатор по умолчанию = "standard")
	textFieldMapping := bleve.NewTextFieldMapping()
	textFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("text", textFieldMapping)

	// Поле source – keyword (без анализа)
	sourceFieldMapping := bleve.NewTextFieldMapping()
	sourceFieldMapping.Analyzer = "keyword"
	sourceFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("source", sourceFieldMapping)

	// Поле summary – текстовое
	summaryFieldMapping := bleve.NewTextFieldMapping()
	summaryFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("summary", summaryFieldMapping)

	// ID – keyword
	idFieldMapping := bleve.NewTextFieldMapping()
	idFieldMapping.Analyzer = "keyword"
	idFieldMapping.Store = true
	docMapping.AddFieldMappingsAt("id", idFieldMapping)

	indexMapping.DefaultMapping = docMapping
	indexMapping.DefaultAnalyzer = "standard" // анализатор по умолчанию для текстовых полей
	return indexMapping
}

// IndexDoc добавляет документ в индекс (используем map для точного соответствия полей)
func (idx *Indexer) IndexDoc(doc Document) error {
	data := map[string]interface{}{
		"id":      doc.ID,
		"text":    doc.Text,
		"source":  doc.Source,
		"date":    doc.Date,
		"markers": doc.Markers,
		"topics":  doc.Topics,
		"summary": doc.Summary,
	}
	return idx.index.Index(doc.ID, data)
}

// Search выполняет поиск и возвращает результат Bleve
func (idx *Indexer) Search(queryString string, limit int) ([]*bleve.SearchResult, error) {
	query := bleve.NewQueryStringQuery(queryString)
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Size = limit
	searchRequest.Fields = []string{"id", "source", "text", "summary"}
	searchRequest.Highlight = bleve.NewHighlight()
	searchResult, err := idx.index.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	return []*bleve.SearchResult{searchResult}, nil
}

// Close закрывает индекс
func (idx *Indexer) Close() error {
	return idx.index.Close()
}

// ConvertResultToDocuments конвертирует результат поиска в список Document
func ConvertResultToDocuments(result *bleve.SearchResult) []Document {
	var docs []Document
	for _, hit := range result.Hits {
		doc := Document{
			ID:     getStringField(hit.Fields, "id"),
			Text:   getStringField(hit.Fields, "text"),
			Source: getStringField(hit.Fields, "source"),
			Summary: getStringField(hit.Fields, "summary"),
		}
		// Если текст не получен через Fields, пытаемся взять из фрагментов
		if doc.Text == "" && len(hit.Fragments) > 0 {
			if textFrags, ok := hit.Fragments["text"]; ok && len(textFrags) > 0 {
				doc.Text = textFrags[0]
			}
		}
		docs = append(docs, doc)
	}
	return docs
}

func getStringField(fields map[string]interface{}, key string) string {
	if val, ok := fields[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}