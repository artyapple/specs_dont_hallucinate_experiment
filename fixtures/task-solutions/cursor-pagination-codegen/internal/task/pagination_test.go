package task

import (
	"context"
	"errors"
	"testing"
	"time"
)

type paginationRepositoryStub struct {
	items    []Task
	limit    int
	position *PagePosition
}

func (*paginationRepositoryStub) Create(context.Context, string, string) (Task, error) {
	return Task{}, nil
}

func (*paginationRepositoryStub) Get(context.Context, string) (Task, error) {
	return Task{}, nil
}

func (r *paginationRepositoryStub) List(_ context.Context, limit int, position *PagePosition) ([]Task, error) {
	r.limit = limit
	r.position = position
	return r.items, nil
}

func (*paginationRepositoryStub) Delete(context.Context, string) error { return nil }

func TestPaginationRoundTripsCursor(t *testing.T) {
	createdAt := time.Date(2026, 8, 17, 12, 0, 0, 123456000, time.UTC)
	repository := &paginationRepositoryStub{items: []Task{
		{ID: "00000000-0000-4000-8000-000000000001", CreatedAt: createdAt},
		{ID: "00000000-0000-4000-8000-000000000002", CreatedAt: createdAt},
	}}
	service := NewService(repository)
	limit := 1
	page, err := service.List(context.Background(), &limit, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 1 || page.NextCursor == "" || repository.limit != 2 {
		t.Fatalf("page = %#v, repository limit = %d", page, repository.limit)
	}
	repository.items = nil
	if _, err := service.List(context.Background(), &limit, &page.NextCursor); err != nil {
		t.Fatalf("List(cursor) error = %v", err)
	}
	if repository.position == nil || repository.position.ID != page.Items[0].ID || !repository.position.CreatedAt.Equal(createdAt) {
		t.Fatalf("decoded position = %#v", repository.position)
	}
}

func TestPaginationRejectsInvalidInputs(t *testing.T) {
	service := NewService(&paginationRepositoryStub{})
	for _, limit := range []int{0, 101} {
		if _, err := service.List(context.Background(), &limit, nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("List(limit %d) error = %v", limit, err)
		}
	}
	invalidCursor := "not-a-cursor"
	if _, err := service.List(context.Background(), nil, &invalidCursor); !errors.Is(err, ErrInvalid) {
		t.Fatalf("List(cursor) error = %v", err)
	}
}
