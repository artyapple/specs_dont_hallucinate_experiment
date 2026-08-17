package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"specs-dont-hallucinate/taskservice/internal/task"
)

const maxRequestBody = 1 << 20

type Service interface {
	Create(context.Context, string) (task.Task, error)
	Get(context.Context, string) (task.Task, error)
	List(context.Context) ([]task.Task, error)
	Delete(context.Context, string) error
	Patch(context.Context, string, task.Patch) (task.Task, error)
}

type Handler struct {
	service Service
}

type createTaskRequest struct {
	Title string `json:"title"`
}

type taskResponse struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	CreatedAt string  `json:"createdAt"`
	DueAt     *string `json:"dueAt"`
}

type patchTaskRequest struct {
	Title json.RawMessage `json:"title"`
	DueAt json.RawMessage `json:"dueAt"`
}

type taskListResponse struct {
	Items []taskResponse `json:"items"`
}

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func RegisterRoutes(router chi.Router, service Service) {
	handler := &Handler{service: service}
	router.Post("/tasks", handler.createTask)
	router.Get("/tasks", handler.listTasks)
	router.Get("/tasks/{taskId}", handler.getTask)
	router.Delete("/tasks/{taskId}", handler.deleteTask)
	router.Patch("/tasks/{taskId}", handler.patchTask)
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeProblem(w, validationProblem())
		return
	}
	var request createTaskRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, validationProblem())
		return
	}
	result, err := h.service.Create(r.Context(), request.Title)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, responseFromTask(result))
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Get(r.Context(), chi.URLParam(r, "taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, responseFromTask(result))
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response := taskListResponse{Items: make([]taskResponse, len(items))}
	for index, item := range items {
		response.Items[index] = responseFromTask(item)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "taskId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) patchTask(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeProblem(w, validationProblem())
		return
	}
	var request patchTaskRequest
	if err := decodeJSON(w, r, &request); err != nil || (request.Title == nil && request.DueAt == nil) {
		writeProblem(w, validationProblem())
		return
	}
	patch := task.Patch{DueAtPresent: request.DueAt != nil}
	if request.Title != nil {
		var title string
		if err := json.Unmarshal(request.Title, &title); err != nil {
			writeProblem(w, validationProblem())
			return
		}
		patch.Title = &title
	}
	if request.DueAt != nil && !bytes.Equal(bytes.TrimSpace(request.DueAt), []byte("null")) {
		var value string
		if err := json.Unmarshal(request.DueAt, &value); err != nil {
			writeProblem(w, validationProblem())
			return
		}
		dueAt, err := time.Parse(time.RFC3339, value)
		if err != nil {
			writeProblem(w, validationProblem())
			return
		}
		patch.DueAt = &dueAt
	}
	result, err := h.service.Patch(r.Context(), chi.URLParam(r, "taskId"), patch)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, responseFromTask(result))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func responseFromTask(value task.Task) taskResponse {
	response := taskResponse{
		ID:        value.ID,
		Title:     value.Title,
		CreatedAt: value.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z"),
	}
	if value.DueAt != nil {
		dueAt := value.DueAt.UTC().Format("2006-01-02T15:04:05.000000Z")
		response.DueAt = &dueAt
	}
	return response
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, task.ErrInvalid):
		writeProblem(w, validationProblem())
	case errors.Is(err, task.ErrNotFound):
		writeProblem(w, notFoundProblem())
	default:
		writeProblem(w, internalProblem())
	}
}

func validationProblem() problem {
	return problem{"urn:problem:validation", "Validation failed", http.StatusBadRequest, "The request is invalid."}
}

func notFoundProblem() problem {
	return problem{"urn:problem:not-found", "Task not found", http.StatusNotFound, "The requested task does not exist."}
}

func internalProblem() problem {
	return problem{"urn:problem:internal", "Internal server error", http.StatusInternalServerError, "The server could not complete the request."}
}

func writeProblem(w http.ResponseWriter, value problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(value.Status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
