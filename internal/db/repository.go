package db

import (
	"liveops/api"
	"log"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type Event struct {
    ID          string                 `gorm:"primaryKey"`
    Title       string
    Description string
    StartTime   *timestamppb.Timestamp `gorm:"type:integer"` // Store as Unix timestamp
    EndTime     *timestamppb.Timestamp `gorm:"type:integer"` // Store as Unix timestamp
    Rewards     string
}

type EventRepository interface {
    GetActiveEvents() ([]Event, error)
    GetEvent(id string) (Event, error)
    CreateEvent(e Event) error
    UpdateEvent(e Event) error
    DeleteEvent(id string) error
    ListEvents() ([]Event, error)
}

type eventRepository struct {
    db *gorm.DB
}

func NewEventRepository(db *gorm.DB) EventRepository {
    if err := db.AutoMigrate(&Event{}); err != nil {
        log.Fatalf("Failed to migrate events table: %v", err)
    }
    return &eventRepository{db: db}
}

func (r *eventRepository) GetActiveEvents() ([]Event, error) {
    now := time.Now().Unix()
    var events []Event
    if err := r.db.Where("start_time <= ? AND end_time >= ?", now, now).Find(&events).Error; err != nil {
        return nil, err
    }
    return events, nil
}

func (r *eventRepository) GetEvent(id string) (Event, error) {
    var e Event
    if err := r.db.First(&e, "id = ?", id).Error; err != nil {
        return Event{}, err
    }
    return e, nil
}

func (r *eventRepository) CreateEvent(e Event) error {
    return r.db.Create(&e).Error
}

func (r *eventRepository) UpdateEvent(e Event) error {
    return r.db.Model(&Event{}).Where("id = ?", e.ID).Updates(e).Error
}

func (r *eventRepository) DeleteEvent(id string) error {
    return r.db.Delete(&Event{}, "id = ?", id).Error
}

func (r *eventRepository) ListEvents() ([]Event, error) {
    var events []Event
    if err := r.db.Find(&events).Error; err != nil {
        return nil, err
    }
    return events, nil
}

// ToProto converts Event to api.EventResponse
func (e Event) ToProto() *api.EventResponse {
    return &api.EventResponse{
        Id:          e.ID,
        Title:       e.Title,
        Description: e.Description,
        StartTime:   e.StartTime,
        EndTime:     e.EndTime,
        Rewards:     e.Rewards,
    }
}