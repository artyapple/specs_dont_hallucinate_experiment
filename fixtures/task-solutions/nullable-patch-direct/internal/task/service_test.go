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
	patchValue   Patch
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

func (r *repositoryStub) List(context.Context) ([]Task, error) {
	return []Task{}, nil
}

func (r *repositoryStub) Delete(context.Context, string) error {
	return r.deleteError
}

func (r *repositoryStub) Patch(_ context.Context, _ string, patch Patch) (Task, error) {
	r.patchValue = patch
	return Task{}, nil
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

func TestPatchTrimsTitleAndPreservesDueAtPresence(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	title := "  changed  "
	if _, err := service.Patch(context.Background(), "123e4567-e89b-42d3-a456-426614174000", Patch{Title: &title, DueAtPresent: true}); err != nil {
		t.Fatalf("Patch() error = %v", err)
	}
	if repository.patchValue.Title == nil || *repository.patchValue.Title != "changed" || !repository.patchValue.DueAtPresent {
		t.Fatalf("repository patch = %#v", repository.patchValue)
	}
}
