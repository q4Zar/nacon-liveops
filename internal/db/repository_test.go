package db

import (
    "database/sql"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "google.golang.org/protobuf/types/known/timestamppb"
    _ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open test db: %v", err)
    }
    _, err = db.Exec(`
        CREATE TABLE events (
            id TEXT PRIMARY KEY,
            title TEXT,
            description TEXT,
            start_time INTEGER,
            end_time INTEGER,
            rewards TEXT
        )`)
    if err != nil {
        t.Fatalf("failed to create table: %v", err)
    }
    return db
}

func TestEventRepository(t *testing.T) {
    db := setupTestDB(t)
    repo := NewEventRepository(db)
    now := time.Now()

    // Test CreateEvent
    event := Event{
        ID:          "1",
        Title:       "Test Event",
        Description: "Test Desc",
        StartTime:   timestamppb.New(now),
        EndTime:     timestamppb.New(now.Add(time.Hour)),
        Rewards:     `{"gold": 100}`,
    }
    assert.NoError(t, repo.CreateEvent(event))

    // Test GetActiveEvents
    events, err := repo.GetActiveEvents()
    assert.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, "Test Event", events[0].Title)

    // Test GetEvent
    retrieved, err := repo.GetEvent("1")
    assert.NoError(t, err)
    assert.Equal(t, "Test Event", retrieved.Title)

    // Test UpdateEvent
    event.Title = "Updated Event"
    assert.NoError(t, repo.UpdateEvent(event))
    updated, err := repo.GetEvent("1")
    assert.NoError(t, err)
    assert.Equal(t, "Updated Event", updated.Title)

    // Test ListEvents
    allEvents, err := repo.ListEvents()
    assert.NoError(t, err)
    assert.Len(t, allEvents, 1)

    // Test DeleteEvent
    assert.NoError(t, repo.DeleteEvent("1"))
    _, err = repo.GetEvent("1")
    assert.Equal(t, sql.ErrNoRows, err)
}