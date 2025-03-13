package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Auto migrate the schema
	if err := db.AutoMigrate(&Event{}); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
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
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)

	// Test ListEvents after deletion
	allEvents, err = repo.ListEvents()
	assert.NoError(t, err)
	assert.Empty(t, allEvents, "ListEvents should return empty slice after deletion")
}
