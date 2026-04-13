package store

import (
	"context"
	"os"
	"testing"
	"time"

	db "github.com/JonMukaj/go-producer-consumer/internal/db/generated"
)

// TestStore_Integration runs against a real Postgres instance.
// Skip with: go test -short ./...
// Run with:  DB_URL=postgres://... go test ./internal/db/...
func TestStore_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	connStr := os.Getenv("DB_URL")
	if connStr == "" {
		connStr = "host=localhost port=5432 user=postgres password=postgres dbname=tasks sslmode=disable"
	}

	ctx := context.Background()
	s, err := New(ctx, connStr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	now := float64(time.Now().UnixNano()) / 1e9

	// Create a task.
	task, err := s.CreateTask(ctx, db.CreateTaskParams{
		Type:         3,
		Value:        42,
		CreationTime: now,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.State != db.TaskStateReceived {
		t.Errorf("want state=received, got %s", task.State)
	}

	// Transition to processing.
	if err := s.UpdateTaskState(ctx, db.UpdateTaskStateParams{
		ID:             task.ID,
		State:          db.TaskStateProcessing,
		LastUpdateTime: float64(time.Now().UnixNano()) / 1e9,
	}); err != nil {
		t.Fatalf("UpdateTaskState processing: %v", err)
	}

	// Transition to done.
	if err := s.UpdateTaskState(ctx, db.UpdateTaskStateParams{
		ID:             task.ID,
		State:          db.TaskStateDone,
		LastUpdateTime: float64(time.Now().UnixNano()) / 1e9,
	}); err != nil {
		t.Fatalf("UpdateTaskState done: %v", err)
	}

	// Verify final state.
	got, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.State != db.TaskStateDone {
		t.Errorf("want state=done, got %s", got.State)
	}

	// CountByStateValue: should be at least 1 done task.
	count, err := s.CountByStateValue(ctx, db.TaskStateDone)
	if err != nil {
		t.Fatalf("CountByStateValue: %v", err)
	}
	if count < 1 {
		t.Errorf("want count>=1, got %d", count)
	}
}
