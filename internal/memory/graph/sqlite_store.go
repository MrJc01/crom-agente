package graph

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir sqlite: %w", err)
	}

	// Create table if not exists
	schema := `
	CREATE TABLE IF NOT EXISTS knowledge_graph (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subject TEXT NOT NULL,
		predicate TEXT NOT NULL,
		object TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_subject ON knowledge_graph(subject);
	CREATE INDEX IF NOT EXISTS idx_object ON knowledge_graph(object);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("falha ao criar schema sqlite: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) SaveTriplet(ctx context.Context, t Triplet) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO knowledge_graph (subject, predicate, object) VALUES (?, ?, ?)", t.Subject, t.Predicate, t.Object)
	return err
}

func (s *SQLiteStore) Search(ctx context.Context, query string) ([]Triplet, error) {
	// Escape LIKE metacharacters
	escapedQuery := strings.ReplaceAll(query, "\\", "\\\\")
	escapedQuery = strings.ReplaceAll(escapedQuery, "%", "\\%")
	escapedQuery = strings.ReplaceAll(escapedQuery, "_", "\\_")

	q := "%" + strings.ToLower(escapedQuery) + "%"
	rows, err := s.db.QueryContext(ctx, "SELECT subject, predicate, object FROM knowledge_graph WHERE lower(subject) LIKE ? ESCAPE '\\' OR lower(predicate) LIKE ? ESCAPE '\\' OR lower(object) LIKE ? ESCAPE '\\'", q, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Triplet
	for rows.Next() {
		var t Triplet
		if err := rows.Scan(&t.Subject, &t.Predicate, &t.Object); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
