package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type repositoryStub struct {
	createdID    string
	createdTitle string
	createResult Task
	getResult    Task
	getError     error
	deleteError  error
	listResult   []Task
	listLimit    int
	listPosition *PagePosition
}

func (r *repositoryStub) Create(_ context.Context, id, title string) (Task, error) {
	r.createdID = id
	r.createdTitle = title
	result := r.createResult
	result.ID = id
	result.Title = title
	return result, nil
}

func (r *repositoryStub) Get(_ context.Context, _ string) (Task, error) {
	return r.getResult, r.getError
}

func (r *repositoryStub) List(_ context.Context, limit int, position *PagePosition) ([]Task, error) {
	r.listLimit = limit
	r.listPosition = position
	return r.listResult, nil
}

func TestListPaginatesAndRoundTripsCursor(t *testing.T) {
	createdAt := time.Date(2026, 8, 17, 12, 0, 0, 123456000, time.UTC)
	repository := &repositoryStub{listResult: []Task{
		{ID: "00000000-0000-4000-8000-000000000001", CreatedAt: createdAt},
		{ID: "00000000-0000-4000-8000-000000000002", CreatedAt: createdAt},
	}}
	service := NewService(repository)
	limit := 1
	page, err := service.List(context.Background(), &limit, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 1 || page.NextCursor == "" || repository.listLimit != 2 {
		t.Fatalf("page = %#v, repository limit = %d", page, repository.listLimit)
	}
	repository.listResult = nil
	if _, err := service.List(context.Background(), &limit, &page.NextCursor); err != nil {
		t.Fatalf("List(cursor) error = %v", err)
	}
	if repository.listPosition == nil || repository.listPosition.ID != page.Items[0].ID || !repository.listPosition.CreatedAt.Equal(createdAt) {
		t.Fatalf("decoded position = %#v", repository.listPosition)
	}
}

func TestListRejectsInvalidInputs(t *testing.T) {
	service := NewService(&repositoryStub{})
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

func (r *repositoryStub) Delete(context.Context, string) error {
	return r.deleteError
}

func TestCreateTrimsTitleAndGeneratesUUID(t *testing.T) {
	repository := &repositoryStub{createResult: Task{CreatedAt: time.Now()}}
	service := NewService(repository)

	created, err := service.Create(context.Background(), "  task title\n")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Title != "task title" || repository.createdTitle != "task title" {
		t.Fatalf("created title = %q, repository title = %q", created.Title, repository.createdTitle)
	}
	if !validUUID(created.ID) || created.ID != repository.createdID {
		t.Fatalf("generated ID = %q", created.ID)
	}
}

func TestCreateValidatesUnicodeCodePoints(t *testing.T) {
	service := NewService(&repositoryStub{})
	for _, title := range []string{"", " \n\t ", strings.Repeat("界", 201)} {
		if _, err := service.Create(context.Background(), title); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Create(%q) error = %v, want ErrInvalid", title, err)
		}
	}
	if _, err := service.Create(context.Background(), strings.Repeat("界", 200)); err != nil {
		t.Fatalf("Create(200 code points) error = %v", err)
	}
}

func TestGetAndDeleteRejectMalformedUUID(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Get(context.Background(), "not-a-uuid"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Get() error = %v, want ErrInvalid", err)
	}
	if err := service.Delete(context.Background(), "not-a-uuid"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Delete() error = %v, want ErrInvalid", err)
	}
}
