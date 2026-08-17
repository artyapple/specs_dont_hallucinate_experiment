package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specs-dont-hallucinate/taskservice/internal/task"
)

type serviceStub struct {
	createResult task.Task
	createError  error
	getResult    task.Task
	getError     error
	listResult   []task.Task
	listError    error
	deleteError  error
	createCalls  int
}

func (s *serviceStub) Create(context.Context, string) (task.Task, error) {
	s.createCalls++
	return s.createResult, s.createError
}

func (s *serviceStub) Get(context.Context, string) (task.Task, error) {
	return s.getResult, s.getError
}

func (s *serviceStub) List(context.Context, *int, *string) (task.Page, error) {
	return task.Page{Items: s.listResult}, s.listError
}

func (s *serviceStub) Delete(context.Context, string) error {
	return s.deleteError
}

func testRouter(service Service) http.Handler {
	router := chi.NewRouter()
	RegisterRoutes(router, service)
	return router
}

func TestCreateTaskResponse(t *testing.T) {
	service := &serviceStub{createResult: task.Task{
		ID:        "123e4567-e89b-42d3-a456-426614174000",
		Title:     "task title",
		CreatedAt: time.Date(2026, 8, 17, 12, 13, 14, 123456000, time.FixedZone("offset", 3600)),
	}}
	request := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBufferString(`{"title":"task title"}`))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response := httptest.NewRecorder()

	testRouter(service).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
	var body taskResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.CreatedAt != "2026-08-17T11:13:14.123456Z" {
		t.Fatalf("createdAt = %q", body.CreatedAt)
	}
}

func TestCreateTaskRejectsUnknownField(t *testing.T) {
	service := &serviceStub{}
	request := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBufferString(`{"title":"task","extra":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	testRouter(service).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
	if service.createCalls != 0 {
		t.Fatalf("Create() calls = %d", service.createCalls)
	}
	if response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
	var body problem
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body != validationProblem() {
		t.Fatalf("problem = %#v", body)
	}
}

func TestGetTaskNotFoundProblem(t *testing.T) {
	service := &serviceStub{getError: task.ErrNotFound}
	request := httptest.NewRequest(http.MethodGet, "/tasks/123e4567-e89b-42d3-a456-426614174000", nil)
	response := httptest.NewRecorder()

	testRouter(service).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
	var body problem
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body != notFoundProblem() {
		t.Fatalf("problem = %#v", body)
	}
}

func TestListTasksReturnsEmptyArray(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	response := httptest.NewRecorder()

	testRouter(&serviceStub{listResult: []task.Task{}}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Body.String() != "{\"items\":[]}\n" {
		t.Fatalf("body = %q", response.Body.String())
	}
}
