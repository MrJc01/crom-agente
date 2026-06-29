package graph

import "context"

// Triplet representa uma relação (Fato) extraída
type Triplet struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

// Store define a interface para o banco de dados do Grafo de Conhecimento (Fase 3)
type Store interface {
	SaveTriplet(ctx context.Context, t Triplet) error
	Search(ctx context.Context, query string) ([]Triplet, error)
	Close() error
}

func NewStore(storageType, path string) (Store, error) {
	if storageType == "sqlite_disk" {
		return NewSQLiteStore(path)
	}
	// default json_file
	return NewJSONStore(path)
}
