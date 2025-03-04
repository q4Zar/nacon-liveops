package db

import (
    "database/sql"
    "log"
    "time"

    "google.golang.org/protobuf/types/known/timestamppb"
)

type Event struct {
    ID          string
    Title       string
    Description string
    StartTime   *timestamppb.Timestamp
    EndTime     *timestamppb.Timestamp
    Rewards     string
}

type EventRepository struct {
    db      *sql.DB
    writeCh chan func() error
}

func NewEventRepository(db *sql.DB) *EventRepository {
    repo := &EventRepository{
        db:      db,
        writeCh: make(chan func() error, 100),
    }
    go repo.processWrites()
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS events (
            id TEXT PRIMARY KEY,
            title TEXT,
            description TEXT,
            start_time INTEGER,
            end_time INTEGER,
            rewards TEXT
        )
    `)
    if err != nil {
        panic(err)
    }
    return repo
}

func (r *EventRepository) processWrites() {
    for fn := range r.writeCh {
        if err := fn(); err != nil {
            log.Printf("write error: %v", err)
        }
    }
}

func (r *EventRepository) GetActiveEvents() ([]Event, error) {
    now := time.Now().Unix()
    rows, err := r.db.Query(`
        SELECT id, title, description, start_time, end_time, rewards 
        FROM events 
        WHERE start_time <= ? AND end_time >= ?`,
        now, now)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var events []Event
    for rows.Next() {
        var e Event
        var startTime, endTime int64
        if err := rows.Scan(&e.ID, &e.Title, &e.Description, &startTime, &e.EndTime, &e.Rewards); err != nil {
            return nil, err
        }
        e.StartTime = timestamppb.New(time.Unix(startTime, 0))
        e.EndTime = timestamppb.New(time.Unix(endTime, 0))
        events = append(events, e)
    }
    return events, nil
}

func (r *EventRepository) GetEvent(id string) (Event, error) {
    var e Event
    var startTime, endTime int64
    err := r.db.QueryRow(`
        SELECT id, title, description, start_time, end_time, rewards 
        FROM events WHERE id = ?`, id).
        Scan(&e.ID, &e.Title, &e.Description, &startTime, &e.EndTime, &e.Rewards)
    if err != nil {
        return Event{}, err
    }
    e.StartTime = timestamppb.New(time.Unix(startTime, 0))
    e.EndTime = timestamppb.New(time.Unix(endTime, 0))
    return e, nil
}

func (r *EventRepository) CreateEvent(e Event) error {
    done := make(chan error, 1)
    r.writeCh <- func() error {
        _, err := r.db.Exec(`
            INSERT INTO events (id, title, description, start_time, end_time, rewards)
            VALUES (?, ?, ?, ?, ?, ?)`,
            e.ID, e.Title, e.Description, e.StartTime.AsTime().Unix(), e.EndTime.AsTime().Unix(), e.Rewards)
        done <- err
        return err
    }
    return <-done
}

// Update other write methods (UpdateEvent, DeleteEvent) similarly