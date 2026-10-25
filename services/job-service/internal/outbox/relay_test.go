package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"image-platform/job-service/internal/outbox"
	"image-platform/job-service/internal/repository"
)

// ── Mock types ────────────────────────────────────────────────────────────────

type mockOutboxRepo struct {
	events    []repository.OutboxEvent
	fetchErr  error
	markCalls []uuid.UUID
	markErr   error
}

func (m *mockOutboxRepo) FetchUnpublished(_ context.Context, _ int) ([]repository.OutboxEvent, error) {
	return m.events, m.fetchErr
}

func (m *mockOutboxRepo) MarkPublished(_ context.Context, id uuid.UUID) error {
	m.markCalls = append(m.markCalls, id)
	return m.markErr
}

type mockPublisher struct {
	published  [][]byte
	publishErr error
}

func (m *mockPublisher) Publish(_ context.Context, body []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.published = append(m.published, body)
	return nil
}

func (m *mockPublisher) Close() error { return nil }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestPublishBatch_PublishesAndMarks(t *testing.T) {
	ev1 := repository.OutboxEvent{ID: uuid.New(), JobID: uuid.New(), Payload: []byte(`{"job_id":"a"}`)}
	ev2 := repository.OutboxEvent{ID: uuid.New(), JobID: uuid.New(), Payload: []byte(`{"job_id":"b"}`)}

	outboxRepo := &mockOutboxRepo{events: []repository.OutboxEvent{ev1, ev2}}
	pub := &mockPublisher{}
	relay := outbox.NewRelay(outboxRepo, pub, 500*time.Millisecond, discardLogger())

	relay.PublishBatch(context.Background())

	assert.Len(t, pub.published, 2)
	assert.Len(t, outboxRepo.markCalls, 2)
	assert.Equal(t, ev1.ID, outboxRepo.markCalls[0])
	assert.Equal(t, ev2.ID, outboxRepo.markCalls[1])
}

func TestPublishBatch_PublishError_SkipsMarkPublished(t *testing.T) {
	ev := repository.OutboxEvent{ID: uuid.New(), JobID: uuid.New(), Payload: []byte(`{"job_id":"a"}`)}
	outboxRepo := &mockOutboxRepo{events: []repository.OutboxEvent{ev}}
	pub := &mockPublisher{publishErr: errors.New("broker unavailable")}
	relay := outbox.NewRelay(outboxRepo, pub, 500*time.Millisecond, discardLogger())

	relay.PublishBatch(context.Background())

	assert.Len(t, pub.published, 0)
	assert.Len(t, outboxRepo.markCalls, 0)
}

func TestPublishBatch_FetchError_NoPublish(t *testing.T) {
	outboxRepo := &mockOutboxRepo{fetchErr: errors.New("db down")}
	pub := &mockPublisher{}
	relay := outbox.NewRelay(outboxRepo, pub, 500*time.Millisecond, discardLogger())

	relay.PublishBatch(context.Background())

	assert.Len(t, pub.published, 0)
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	outboxRepo := &mockOutboxRepo{}
	pub := &mockPublisher{}
	relay := outbox.NewRelay(outboxRepo, pub, 10*time.Millisecond, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		relay.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// pass
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}
