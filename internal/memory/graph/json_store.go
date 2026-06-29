package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type JSONStore struct {
	mu       sync.RWMutex
	filePath string
	data     []Triplet
}

func NewJSONStore(filePath string) (*JSONStore, error) {
	s := &JSONStore{
		filePath: filePath,
		data:     make([]Triplet, 0),
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("falha ao carregar grafo json: %w", err)
	}
	return s, nil
}

func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bytes, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	if len(bytes) == 0 {
		return nil
	}
	return json.Unmarshal(bytes, &s.data)
}

func (s *JSONStore) save(dataCopy []Triplet) error {
	bytes, err := json.MarshalIndent(dataCopy, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, bytes, 0644)
}

func (s *JSONStore) SaveTriplet(ctx context.Context, t Triplet) error {
	s.mu.Lock()
	s.data = append(s.data, t)
	dataCopy := make([]Triplet, len(s.data))
	copy(dataCopy, s.data)
	s.mu.Unlock()

	return s.save(dataCopy)
}

func (s *JSONStore) Search(ctx context.Context, query string) ([]Triplet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Triplet
	q := strings.ToLower(query)
	for _, t := range s.data {
		if strings.Contains(strings.ToLower(t.Subject), q) ||
			strings.Contains(strings.ToLower(t.Predicate), q) ||
			strings.Contains(strings.ToLower(t.Object), q) {
			results = append(results, t)
		}
	}
	return results, nil
}

func (s *JSONStore) Close() error {
	return nil
}
