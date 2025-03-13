package event

import (
	"context"
	"database/sql"
	"liveops/api"
	"liveops/internal/db"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Ensure MockEventRepository implements db.EventRepository
var _ db.EventRepository = (*MockEventRepository)(nil)

// MockEventRepository implements db.EventRepository
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

func TestService(t *testing.T) {
	mockRepo := new(MockEventRepository)
	logger := zap.NewNop()
	svc := NewService(mockRepo, logger)

	ctx := context.Background()
	now := time.Now()

	sampleEvent := db.Event{
		ID:          "evt1",
		Title:       "Test Event",
		Description: "A test event",
		StartTime:   timestamppb.New(now),
		EndTime:     timestamppb.New(now.Add(time.Hour)),
		Rewards:     `{"gold": 100}`,
	}

	t.Run("GetActiveEvents_Success", func(t *testing.T) {
		mockRepo.On("GetActiveEvents").Return([]db.Event{sampleEvent}, nil).Once()

		resp, err := svc.GetActiveEvents(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Events, 1)
		assert.Equal(t, "Test Event", resp.Events[0].Title)
		mockRepo.AssertExpectations(t)
	})

	t.Run("GetActiveEvents_Error", func(t *testing.T) {
		mockRepo.On("GetActiveEvents").Return([]db.Event{}, sql.ErrConnDone).Once()

		resp, err := svc.GetActiveEvents(ctx)
		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("GetEvent_Success", func(t *testing.T) {
		mockRepo.On("GetEvent", "evt1").Return(sampleEvent, nil).Once()

		event, err := svc.GetEvent(ctx, "evt1")
		assert.NoError(t, err)
		assert.Equal(t, "Test Event", event.Title)
		mockRepo.AssertExpectations(t)
	})

	t.Run("GetEvent_NotFound", func(t *testing.T) {
		mockRepo.On("GetEvent", "evt2").Return(db.Event{}, sql.ErrNoRows).Once()

		event, err := svc.GetEvent(ctx, "evt2")
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.Empty(t, event)
		mockRepo.AssertExpectations(t)
	})

	t.Run("CreateEvent_Success", func(t *testing.T) {
		req := &api.EventRequest{
			Id:          "evt1",
			Title:       "Test Event",
			Description: "A test event",
			StartTime:   now.Unix(),
			EndTime:     now.Add(time.Hour).Unix(),
			Rewards:     `{"gold": 100}`,
		}
		mockRepo.On("CreateEvent", mock.AnythingOfType("db.Event")).Return(nil).Once()

		resp, err := svc.CreateEvent(ctx, req)
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
			StartTime:   now.Unix(),
			EndTime:     now.Add(time.Hour).Unix(),
			Rewards:     `{"gold": 100}`,
		}
		mockRepo.On("CreateEvent", mock.AnythingOfType("db.Event")).Return(sql.ErrConnDone).Once()

		resp, err := svc.CreateEvent(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("UpdateEvent_Success", func(t *testing.T) {
		req := &api.EventRequest{
			Id:          "evt1",
			Title:       "Updated Event",
			Description: "Updated desc",
			StartTime:   now.Unix(),
			EndTime:     now.Add(time.Hour).Unix(),
			Rewards:     `{"gold": 200}`,
		}
		mockRepo.On("UpdateEvent", mock.AnythingOfType("db.Event")).Return(nil).Once()

		resp, err := svc.UpdateEvent(ctx, req)
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
			StartTime:   now.Unix(),
			EndTime:     now.Add(time.Hour).Unix(),
			Rewards:     `{"gold": 200}`,
		}
		mockRepo.On("UpdateEvent", mock.AnythingOfType("db.Event")).Return(sql.ErrNoRows).Once()

		resp, err := svc.UpdateEvent(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("DeleteEvent_Success", func(t *testing.T) {
		req := &api.DeleteRequest{Id: "evt1"}
		mockRepo.On("DeleteEvent", "evt1").Return(nil).Once()

		resp, err := svc.DeleteEvent(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		mockRepo.AssertExpectations(t)
	})
}
