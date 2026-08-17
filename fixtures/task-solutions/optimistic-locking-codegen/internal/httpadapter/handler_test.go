package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	generated "specs-dont-hallucinate/taskservice/internal/httpapi"
	"specs-dont-hallucinate/taskservice/internal/task"
)

type serviceStub struct {
	createResult task.Task
	createCalls  int
}

func (s *serviceStub) Create(context.Context, string) (task.Task, error) {
	s.createCalls++
	return s.createResult, nil
}

func (*serviceStub) Get(context.Context, string) (task.Task, error) {
	return task.Task{}, task.ErrNotFound
}
func (*serviceStub) List(context.Context) ([]task.Task, error) { return []task.Task{}, nil }
func (*serviceStub) Delete(context.Context, string) error      { return nil }
func (*serviceStub) Update(context.Context, string, string, int64) (task.Task, error) {
	return task.Task{}, nil
}

func testRouter(service Service) http.Handler {
	router := chi.NewRouter()
	RegisterRoutes(router, service)
	return router
}

func TestCreateRejectsUnknownField(t *testing.T) {
	service := &serviceStub{}
	request := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBufferString(`{"title":"task","extra":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	testRouter(service).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || service.createCalls != 0 {
		t.Fatalf("status = %d, create calls = %d", response.Code, service.createCalls)
	}
	assertProblem(t, response, validationProblem())
}

func TestMalformedPathReturnsValidationProblem(t *testing.T) {
	response := httptest.NewRecorder()
	testRouter(&serviceStub{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasks/not-a-uuid", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
	assertProblem(t, response, validationProblem())
}

func TestCreateFormatsTimestamp(t *testing.T) {
	service := &serviceStub{createResult: task.Task{
		ID:        "123e4567-e89b-42d3-a456-426614174000",
		Title:     "task",
		CreatedAt: time.Date(2026, 8, 17, 12, 13, 14, 123456000, time.FixedZone("offset", 3600)),
		Version:   1,
	}}
	request := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBufferString(`{"title":"task"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	testRouter(service).ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body generated.Task
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	if body.CreatedAt != "2026-08-17T11:13:14.123456Z" {
		t.Fatalf("createdAt = %q", body.CreatedAt)
	}
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, expected generated.Problem) {
	t.Helper()
	if response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
	var actual generated.Problem
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if actual != expected {
		t.Fatalf("problem = %#v, want %#v", actual, expected)
	}
}

func TestParseIfMatch(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		version int64
		status  int
		ok      bool
	}{
		{name: "missing", status: http.StatusPreconditionRequired},
		{name: "valid", values: []string{"\t\"7\" "}, version: 7, ok: true},
		{name: "zero", values: []string{`"0"`}, status: http.StatusBadRequest},
		{name: "signed", values: []string{`"+1"`}, status: http.StatusBadRequest},
		{name: "wildcard", values: []string{"*"}, status: http.StatusBadRequest},
		{name: "list", values: []string{`"1", "2"`}, status: http.StatusBadRequest},
		{name: "multiple", values: []string{`"1"`, `"2"`}, status: http.StatusBadRequest},
		{name: "overflow", values: []string{`"9223372036854775808"`}, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, status, ok := parseIfMatch(test.values)
			if version != test.version || status != test.status || ok != test.ok {
				t.Fatalf("parseIfMatch() = (%d, %d, %t), want (%d, %d, %t)", version, status, ok, test.version, test.status, test.ok)
			}
		})
	}
}
