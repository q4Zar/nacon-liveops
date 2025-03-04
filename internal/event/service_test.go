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

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "google.golang.org/protobuf/types/known/timestamppb"
)

// MockEventRepository for testing
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

func TestGetActiveEvents(t *testing.T) {
    mockRepo := new(MockEventRepository)
    svc := NewService(mockRepo)

    events := []db.Event{{
        ID:          "1",
        Title:       "Event 1",
        Description: "Desc 1",
        StartTime:   timestamppb.Now(),
        EndTime:     timestamppb.New(time.Now().Add(time.Hour)),
        Rewards:     `{"gold": 100}`,
    }}
    mockRepo.On("GetActiveEvents").Return(events, nil)

    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/events", nil)
    svc.GetActiveEvents(rr, req)

    assert.Equal(t, http.StatusOK, rr.Code)
    assert.Contains(t, rr.Body.String(), `"title":"Event 1"`)
    mockRepo.AssertExpectations(t)
}

func TestGetEvent(t *testing.T) {
    mockRepo := new(MockEventRepository)
    svc := NewService(mockRepo)

    event := db.Event{
        ID:          "1",
        Title:       "Event 1",
        Description: "Desc 1",
        StartTime:   timestamppb.Now(),
        EndTime:     timestamppb.New(time.Now().Add(time.Hour)),
        Rewards:     `{"gold": 100}`,
    }
    mockRepo.On("GetEvent", "1").Return(event, nil)
    mockRepo.On("GetEvent", "2").Return(db.Event{}, sql.ErrNoRows)

    tests := []struct {
        name           string
        id             string
        expectedStatus int
    }{{
        name:           "Valid Event",
        id:             "1",
        expectedStatus: http.StatusOK,
    }, {
        name:           "Event Not Found",
        id:             "2",
        expectedStatus: http.StatusNotFound,
    }}

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            rr := httptest.NewRecorder()
            req := httptest.NewRequest("GET", "/events/"+tt.id, nil)
            req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, &chi.Context{
                URLParams: map[string]string{"id": tt.id},
            }))
            svc.GetEvent(rr, req)

            assert.Equal(t, tt.expectedStatus, rr.Code)
            if tt.expectedStatus == http.StatusOK {
                assert.Contains(t, rr.Body.String(), `"title":"Event 1"`)
            }
        })
    }
    mockRepo.AssertExpectations(t)
}

func TestCreateEvent(t *testing.T) {
    mockRepo := new(MockEventRepository)
    svc := NewService(mockRepo)

    req := &api.EventRequest{
        Id:          "1",
        Title:       "Event 1",
        Description: "Desc 1",
        StartTime:   timestamppb.Now(),
        EndTime:     timestamppb.New(time.Now().Add(time.Hour)),
        Rewards:     `{"gold": 100}`,
    }
    mockRepo.On("CreateEvent", mock.AnythingOfType("db.Event")).Return(nil)

    resp, err := svc.CreateEvent(context.Background(), req)

    assert.NoError(t, err)
    assert.Equal(t, "Event 1", resp.Title)
    mockRepo.AssertExpectations(t)
}