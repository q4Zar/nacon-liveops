package db

import (
	"log"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Event struct {
	ID          string `gorm:"primaryKey"`
	Title       string
	Description string
	StartTime   *timestamppb.Timestamp `gorm:"-"`
	EndTime     *timestamppb.Timestamp `gorm:"-"`
	StartTimeUnix int64 `gorm:"column:start_time"`
	EndTimeUnix   int64 `gorm:"column:end_time"`
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

func NewEventRepository(dbPath string) EventRepository {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Auto Migrate the schema
	err = db.AutoMigrate(&Event{})
	if err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	return &eventRepository{
		db: db,
	}
}

func (r *eventRepository) GetActiveEvents() ([]Event, error) {
	var events []Event
	now := time.Now().Unix()
	
	err := r.db.Where("start_time <= ? AND end_time >= ?", now, now).Find(&events).Error
	if err != nil {
		return nil, err
	}

	// Convert Unix timestamps to protobuf timestamps
	for i := range events {
		events[i].StartTime = timestamppb.New(time.Unix(events[i].StartTimeUnix, 0))
		events[i].EndTime = timestamppb.New(time.Unix(events[i].EndTimeUnix, 0))
	}

	return events, nil
}

func (r *eventRepository) GetEvent(id string) (Event, error) {
	var event Event
	err := r.db.First(&event, "id = ?", id).Error
	if err != nil {
		return Event{}, err
	}

	event.StartTime = timestamppb.New(time.Unix(event.StartTimeUnix, 0))
	event.EndTime = timestamppb.New(time.Unix(event.EndTimeUnix, 0))
	return event, nil
}

func (r *eventRepository) CreateEvent(e Event) error {
	e.StartTimeUnix = e.StartTime.AsTime().Unix()
	e.EndTimeUnix = e.EndTime.AsTime().Unix()
	return r.db.Create(&e).Error
}

func (r *eventRepository) UpdateEvent(e Event) error {
	e.StartTimeUnix = e.StartTime.AsTime().Unix()
	e.EndTimeUnix = e.EndTime.AsTime().Unix()
	return r.db.Save(&e).Error
}

func (r *eventRepository) DeleteEvent(id string) error {
	return r.db.Delete(&Event{}, "id = ?", id).Error
}

func (r *eventRepository) ListEvents() ([]Event, error) {
	var events []Event
	err := r.db.Find(&events).Error
	if err != nil {
		return nil, err
	}

	// Convert Unix timestamps to protobuf timestamps
	for i := range events {
		events[i].StartTime = timestamppb.New(time.Unix(events[i].StartTimeUnix, 0))
		events[i].EndTime = timestamppb.New(time.Unix(events[i].EndTimeUnix, 0))
	}

	return events, nil
}