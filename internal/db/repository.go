package db

import (
    "database/sql"
    "log"
    "sync"
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

// EventRepository defines the interface for event storage operations
type EventRepository interface {
    GetActiveEvents() ([]Event, error)
    GetEvent(id string) (Event, error)
    CreateEvent(e Event) error
    UpdateEvent(e Event) error
    DeleteEvent(id string) error
    ListEvents() ([]Event, error)
}

type eventRepository struct {
    db       *sql.DB
    writeCh  chan func() error
    readPool *sync.Pool
}

func NewEventRepository(db *sql.DB) EventRepository {
    repo := &eventRepository{
        db:      db,
        writeCh: make(chan func() error, 100),
        readPool: &sync.Pool{
            New: func() interface{} {
                conn, err := sql.Open("sqlite3", "./liveops.db")
                if err != nil {
                    log.Fatalf("failed to open read pool conn: %v", err)
                }
                conn.SetMaxOpenConns(1)
                return conn
            },
        },
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

func (r *eventRepository) processWrites() {
    for fn := range r.writeCh {
        if err := fn(); err != nil {
            log.Printf("write error: %v", err)
        }
    }
}

func (r *eventRepository) GetActiveEvents() ([]Event, error) {
    conn := r.readPool.Get().(*sql.DB)
    defer r.readPool.Put(conn)

    now := time.Now().Unix()
    rows, err := conn.Query(`
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

func (r *eventRepository) GetEvent(id string) (Event, error) {
    conn := r.readPool.Get().(*sql.DB)
    defer r.readPool.Put(conn)

    var e Event
    var startTime, endTime int64
    err := conn.QueryRow(`
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

func (r *eventRepository) CreateEvent(e Event) error {
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

func (r *eventRepository) UpdateEvent(e Event) error {
    done := make(chan error, 1)
    r.writeCh <- func() error {
        _, err := r.db.Exec(`
            UPDATE events 
            SET title = ?, description = ?, start_time = ?, end_time = ?, rewards = ?
            WHERE id = ?`,
            e.Title, e.Description, e.StartTime.AsTime().Unix(), e.EndTime.AsTime().Unix(), e.Rewards, e.ID)
        done <- err
        return err
    }
    return <-done
}

func (r *eventRepository) DeleteEvent(id string) error {
    done := make(chan error, 1)
    r.writeCh <- func() error {
        _, err := r.db.Exec(`DELETE FROM events WHERE id = ?`, id)
        done <- err
        return err
    }
    return <-done
}

func (r *eventRepository) ListEvents() ([]Event, error) {
    conn := r.readPool.Get().(*sql.DB)
    defer r.readPool.Put(conn)

    rows, err := conn.Query(`
        SELECT id, title, description, start_time, end_time, rewards 
        FROM events`)
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