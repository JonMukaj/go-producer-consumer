package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/JonMukaj/go-producer-consumer/internal/db/generated"
)

// Store wraps the sqlc Queries with the underlying connection pool so both
// services can share a single package.
type Store struct {
	*db.Queries
	pool *pgxpool.Pool
}

// New opens a pgx connection pool and returns a ready-to-use Store.
func New(ctx context.Context, connStr string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("open db pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{
		Queries: db.New(pool),
		pool:    pool,
	}, nil
}

// Close releases the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}
