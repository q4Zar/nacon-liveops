package event

import (
    "context"
    "database/sql"
    "liveops/api"
    "liveops/internal/db"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "google.golang.org/protobuf/types/known/timestamppb"
)

// MockEventRepository mocks the db.EventRepository interface
type MockEventRepository struct {
    mock.Mock
}

func (m *MockEventRepository) GetActiveEvents() ([]db.Event, error) {
    args := m.Called()
    return args.Get(0).([]db.Event), args.Error(1)
}

func (m *MockEventRepository) GetEvent(id string) (db.Event, error) {
    args := m.Called(id)
    return args.Get(0).(db.Event), args.Error(1)
}

func (m *MockEventRepository) CreateEvent(e db.Event) error {
    args := m.Called(e)
    return args.Error(0)
}

func (m *MockEventRepository) UpdateEvent(e db.Event) error {
    args := m.Called(e)
    return args.Error(0)
}

func (m *MockEventRepository) DeleteEvent(id string) error {
    args := m.Called(id)
    return args.Error(0)
}

func (m *MockEventRepository) ListEvents() ([]db.Event, error) {
    args := m.Called()
    return args.Get(0).([]db.Event), args.Error(1)
}

// Test Suite
func TestService(t *testing.T) {
    // Setup mock repository
    mockRepo := new(MockEventRepository)
    svc := NewService(mockRepo)

    // Sample event for testing
    sampleEvent := db.Event{
        ID:          "evt1",
        Title:       "Test Event",
        Description: "A test event",
        StartTime:   timestamppb.Now(),
        EndTime:     timestamppb.New(time.Now().Add(time.Hour)),
        Rewards:     `{"gold": 100}`,
    }

    // Test GetActiveEvents
    t.Run("GetActiveEvents_Success", func(t *testing.T) {
        mockRepo.On("GetActiveEvents").Return([]db.Event{sampleEvent}, nil).Once()

        rr := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/events", nil)
        svc.GetActiveEvents(rr, req)

        assert.Equal(t, http.StatusOK, rr.Code)
        assert.Contains(t, rr.Body.String(), `"title":"Test Event"`)
        assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
        mockRepo.AssertExpectations(t)
    })

    t.Run("GetActiveEvents_Error", func(t *testing.T) {
        mockRepo.On("GetActiveEvents").Return([]db.Event{}, sql.ErrConnDone).Once()

        rr := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/events", nil)
        svc.GetActiveEvents(rr, req)

        assert.Equal(t, http.StatusInternalServerError, rr.Code)
        assert.Contains(t, rr.Body.String(), `"error":"internal server error"`)
        mockRepo.AssertExpectations(t)
    })

    // Test GetEvent
    t.Run("GetEvent_Success", func(t *testing.T) {
        mockRepo.On("GetEvent", "evt1").Return(sampleEvent, nil).Once()

        rr := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/events/evt1", nil)
        req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
            URLParams: map[string]string{"id": "evt1"},
        }))
        svc.GetEvent(rr, req)

        assert.Equal(t, http.StatusOK, rr.Code)
        assert.Contains(t, rr.Body.String(), `"title":"Test Event"`)
        mockRepo.AssertExpectations(t)
    })

    t.Run("GetEvent_NotFound", func(t *testing.T) {
        mockRepo.On("GetEvent", "evt2").Return(db.Event{}, sql.ErrNoRows).Once()

        rr := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/events/evt2", nil)
        req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
            URLParams: map[string]string{"id": "evt2"},
        }))
        svc.GetEvent(rr, req)

        assert.Equal(t, http.StatusNotFound, rr.Code)
        assert.Contains(t, rr.Body.String(), `"error":"event not found"`)
        mockRepo.AssertExpectations(t)
    })

    t.Run("GetEvent_InvalidID", func(t *testing.T) {
        rr := httptest.NewRecorder()
        req := httptest.NewRequest("GET", "/events/", nil)
        req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
            URLParams: map[string]string{"id": ""},
        }))
        svc.GetEvent(rr, req)

        assert.Equal(t, http.StatusBadRequest, rr.Code)
        assert.Contains(t, rr.Body.String(), `"error":"missing event id"`)
    })

    // Test CreateEvent
    t.Run("CreateEvent_Success", func(t *testing.T) {
        req := &api.EventRequest{
            Id:          "evt1",
            Title:       "Test Event",
            Description: "A test event",
            StartTime:   sampleEvent.StartTime.AsTime().Unix(),
            EndTime:     sampleEvent.EndTime.AsTime().Unix(),
            Rewards:     `{"gold": 100}`,
        }
        mockRepo.On("CreateEvent", mock.AnythingOfType("db.Event")).Return(nil).Once()

        resp, err := svc.CreateEvent(context.Background(), req)

        assert.NoError(t, err)
        assert.NotNil(t, resp)
        assert.Equal(t, "evt1", resp.Id)
        assert.Equal(t, "Test Event", resp.Title)
        assert.Equal(t, req.StartTime, resp.StartTime)
        mockRepo.AssertExpectations(t)
    })

    t.Run("CreateEvent_Error", func(t *testing.T) {
        req := &api.EventRequest{
            Id:          "evt1",
            Title:       "Test Event",
            Description: "A test event",
            StartTime:   sampleEvent.StartTime.AsTime().Unix(),
            EndTime:     sampleEvent.EndTime.AsTime().Unix(),
            Rewards:     `{"gold": 100}`,
        }
        mockRepo.On("CreateEvent", mock.AnythingOfType("db.Event")).Return(sql.ErrConnDone).Once()

        resp, err := svc.CreateEvent(context.Background(), req)

        assert.Error(t, err)
        assert.Nil(t, resp)
        assert.Equal(t, sql.ErrConnDone, err)
        mockRepo.AssertExpectations(t)
    })

    // Test UpdateEvent
    t.Run("UpdateEvent_Success", func(t *testing.T) {
        req := &api.EventRequest{
            Id:          "evt1",
            Title:       "Updated Event",
            Description: "Updated desc",
            StartTime:   sampleEvent.StartTime.AsTime().Unix(),
            EndTime:     sampleEvent.EndTime.AsTime().Unix(),
            Rewards:     `{"gold": 200}`,
        }
        mockRepo.On("UpdateEvent", mock.AnythingOfType("db.Event")).Return(nil).Once()

        resp, err := svc.UpdateEvent(context.Background(), req)

        assert.NoError(t, err)
        assert.NotNil(t, resp)
        assert.Equal(t, "Updated Event", resp.Title)
        assert.Equal(t, "Updated desc", resp.Description)
        mockRepo.AssertExpectations(t)
    })

    t.Run("UpdateEvent_Error", func(t *testing.T) {
        req := &api.EventRequest{
            Id:          "evt1",
            Title:       "Updated Event",
            Description: "Updated desc",
            StartTime:   sampleEvent.StartTime.AsTime().Unix(),
            EndTime:     sampleEvent.EndTime.AsTime().Unix(),
            Rewards:     `{"gold": 200}`,
        }
        mockRepo.On("UpdateEvent", mock.AnythingOfType("db.Event")).Return(sql.ErrNoRows).Once()

        resp, err := svc.UpdateEvent(context.Background(), req)

        assert.Error(t, err)
        assert.Nil(t, resp)
        assert.Equal(t, sql.ErrNoRows, err)
        mockRepo.AssertExpectations(t)
    })

    // Test DeleteEvent
    t.Run("DeleteEvent_Success", func(t *testing.T) {
        req := &api.DeleteRequest{Id: "evt1"}
        mockRepo.On("DeleteEvent", "evt1").Return(nil).Once()

        resp, err := svc.DeleteEvent(context.Background(), req)

        assert.NoError(t, err)
        assert.NotNil(t, resp)
        mockRepo.AssertExpectations(t)
    })

    t.Run("DeleteEvent_Error", func(t *testing.T) {
        req := &api.DeleteRequest{Id: "evt1"}
        mockRepo.On("DeleteEvent", "evt1").Return(sql.ErrNoRows).Once()

        resp, err := svc.DeleteEvent(context.Background(), req)

        assert.Error(t, err)
        assert.Nil(t, resp)
        assert.Equal(t, sql.ErrNoRows, err)
        mockRepo.AssertExpectations(t)
    })

    // Test ListEvents
    t.Run("ListEvents_Success", func(t *testing.T) {
        mockRepo.On("ListEvents").Return([]db.Event{sampleEvent}, nil).Once()

        resp, err := svc.ListEvents(context.Background(), &api.Empty{})

        assert.NoError(t, err)
        assert.NotNil(t, resp)
        assert.Len(t, resp.Events, 1)
        assert.Equal(t, "Test Event", resp.Events[0].Title)
        assert.Equal(t, sampleEvent.StartTime.AsTime().Unix(), resp.Events[0].StartTime)
        mockRepo.AssertExpectations(t)
    })

    t.Run("ListEvents_Error", func(t *testing.T) {
        mockRepo.On("ListEvents").Return([]db.Event{}, sql.ErrConnDone).Once()

        resp, err := svc.ListEvents(context.Background(), &api.Empty{})

        assert.Error(t, err)
        assert.Nil(t, resp)
        assert.Equal(t, sql.ErrConnDone, err)
        mockRepo.AssertExpectations(t)
    })
}