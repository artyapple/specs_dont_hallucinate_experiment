package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	generated "specs-dont-hallucinate/taskservice/internal/httpapi"
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

type Handler struct{ service Service }

func RegisterRoutes(router chi.Router, service Service) {
	adapter := &Handler{service: service}
	strict := generated.NewStrictHandlerWithOptions(adapter, nil, generated.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  func(w http.ResponseWriter, _ *http.Request, _ error) { writeProblem(w, validationProblem()) },
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) { writeProblem(w, internalProblem()) },
	})
	router.Group(func(router chi.Router) {
		router.Use(strictJSONBody)
		generated.HandlerWithOptions(strict, generated.ChiServerOptions{
			BaseRouter:       router,
			ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) { writeProblem(w, validationProblem()) },
		})
	})
}

func (h *Handler) CreateTask(ctx context.Context, request generated.CreateTaskRequestObject) (generated.CreateTaskResponseObject, error) {
	if request.Body == nil {
		return createValidationResponse(), nil
	}
	result, err := h.service.Create(ctx, request.Body.Title)
	if errors.Is(err, task.ErrInvalid) {
		return createValidationResponse(), nil
	}
	if err != nil {
		return generated.CreateTask500ApplicationProblemPlusJSONResponse{
			InternalProblemApplicationProblemPlusJSONResponse: generated.InternalProblemApplicationProblemPlusJSONResponse(internalProblem()),
		}, nil
	}
	response, err := responseFromTask(result)
	if err != nil {
		return nil, err
	}
	return generated.CreateTask201JSONResponse(response), nil
}

func (h *Handler) GetTask(ctx context.Context, request generated.GetTaskRequestObject) (generated.GetTaskResponseObject, error) {
	result, err := h.service.Get(ctx, request.TaskId.String())
	if errors.Is(err, task.ErrInvalid) {
		return generated.GetTask400ApplicationProblemPlusJSONResponse{
			ValidationProblemApplicationProblemPlusJSONResponse: generated.ValidationProblemApplicationProblemPlusJSONResponse(validationProblem()),
		}, nil
	}
	if errors.Is(err, task.ErrNotFound) {
		return generated.GetTask404ApplicationProblemPlusJSONResponse{
			NotFoundProblemApplicationProblemPlusJSONResponse: generated.NotFoundProblemApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	}
	if err != nil {
		return generated.GetTask500ApplicationProblemPlusJSONResponse{
			InternalProblemApplicationProblemPlusJSONResponse: generated.InternalProblemApplicationProblemPlusJSONResponse(internalProblem()),
		}, nil
	}
	response, err := responseFromTask(result)
	if err != nil {
		return nil, err
	}
	return generated.GetTask200JSONResponse(response), nil
}

func (h *Handler) ListTasks(ctx context.Context, _ generated.ListTasksRequestObject) (generated.ListTasksResponseObject, error) {
	items, err := h.service.List(ctx)
	if err != nil {
		return generated.ListTasks500ApplicationProblemPlusJSONResponse{
			InternalProblemApplicationProblemPlusJSONResponse: generated.InternalProblemApplicationProblemPlusJSONResponse(internalProblem()),
		}, nil
	}
	response := generated.TaskList{Items: make([]generated.Task, len(items))}
	for index, item := range items {
		response.Items[index], err = responseFromTask(item)
		if err != nil {
			return nil, err
		}
	}
	return generated.ListTasks200JSONResponse(response), nil
}

func (h *Handler) DeleteTask(ctx context.Context, request generated.DeleteTaskRequestObject) (generated.DeleteTaskResponseObject, error) {
	err := h.service.Delete(ctx, request.TaskId.String())
	if errors.Is(err, task.ErrInvalid) {
		return generated.DeleteTask400ApplicationProblemPlusJSONResponse{
			ValidationProblemApplicationProblemPlusJSONResponse: generated.ValidationProblemApplicationProblemPlusJSONResponse(validationProblem()),
		}, nil
	}
	if errors.Is(err, task.ErrNotFound) {
		return generated.DeleteTask404ApplicationProblemPlusJSONResponse{
			NotFoundProblemApplicationProblemPlusJSONResponse: generated.NotFoundProblemApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	}
	if err != nil {
		return generated.DeleteTask500ApplicationProblemPlusJSONResponse{
			InternalProblemApplicationProblemPlusJSONResponse: generated.InternalProblemApplicationProblemPlusJSONResponse(internalProblem()),
		}, nil
	}
	return generated.DeleteTask204Response{}, nil
}

func (h *Handler) PatchTask(ctx context.Context, request generated.PatchTaskRequestObject) (generated.PatchTaskResponseObject, error) {
	if request.Body == nil || (request.Body.Title == nil && !request.Body.DueAt.IsSpecified()) {
		return patchValidationResponse(), nil
	}
	patch := task.Patch{Title: request.Body.Title, DueAtPresent: request.Body.DueAt.IsSpecified()}
	if patch.DueAtPresent && !request.Body.DueAt.IsNull() {
		value, err := request.Body.DueAt.Get()
		if err != nil {
			return patchValidationResponse(), nil
		}
		patch.DueAt = &value
	}
	result, err := h.service.Patch(ctx, request.TaskId.String(), patch)
	if errors.Is(err, task.ErrInvalid) {
		return patchValidationResponse(), nil
	}
	if errors.Is(err, task.ErrNotFound) {
		return generated.PatchTask404ApplicationProblemPlusJSONResponse{
			NotFoundProblemApplicationProblemPlusJSONResponse: generated.NotFoundProblemApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	}
	if err != nil {
		return generated.PatchTask500ApplicationProblemPlusJSONResponse{
			InternalProblemApplicationProblemPlusJSONResponse: generated.InternalProblemApplicationProblemPlusJSONResponse(internalProblem()),
		}, nil
	}
	response, err := responseFromTask(result)
	if err != nil {
		return nil, err
	}
	return generated.PatchTask200JSONResponse(response), nil
}

func strictJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isCreate := r.Method == http.MethodPost && r.URL.Path == "/tasks"
		isPatch := r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/tasks/")
		if !isCreate && !isPatch {
			next.ServeHTTP(w, r)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeProblem(w, validationProblem())
			return
		}
		contents, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
		if err != nil || len(contents) > maxRequestBody {
			writeProblem(w, validationProblem())
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(contents))
		decoder.DisallowUnknownFields()
		var body any
		if isCreate {
			body = &generated.CreateTaskRequest{}
		} else {
			body = &generated.PatchTaskRequest{}
		}
		if err := decoder.Decode(body); err != nil {
			writeProblem(w, validationProblem())
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeProblem(w, validationProblem())
			return
		}
		if isPatch {
			var members map[string]json.RawMessage
			if err := json.Unmarshal(contents, &members); err != nil || bytes.Equal(bytes.TrimSpace(members["title"]), []byte("null")) {
				writeProblem(w, validationProblem())
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(contents))
		next.ServeHTTP(w, r)
	})
}

func responseFromTask(value task.Task) (generated.Task, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return generated.Task{}, err
	}
	result := generated.Task{Id: id, Title: value.Title, CreatedAt: value.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z")}
	if value.DueAt == nil {
		result.DueAt.SetNull()
	} else {
		result.DueAt.Set(value.DueAt.UTC().Format("2006-01-02T15:04:05.000000Z"))
	}
	return result, nil
}

func createValidationResponse() generated.CreateTask400ApplicationProblemPlusJSONResponse {
	return generated.CreateTask400ApplicationProblemPlusJSONResponse{
		ValidationProblemApplicationProblemPlusJSONResponse: generated.ValidationProblemApplicationProblemPlusJSONResponse(validationProblem()),
	}
}

func patchValidationResponse() generated.PatchTask400ApplicationProblemPlusJSONResponse {
	return generated.PatchTask400ApplicationProblemPlusJSONResponse{
		ValidationProblemApplicationProblemPlusJSONResponse: generated.ValidationProblemApplicationProblemPlusJSONResponse(validationProblem()),
	}
}

func validationProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:validation", Title: "Validation failed", Status: http.StatusBadRequest, Detail: "The request is invalid."}
}

func notFoundProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:not-found", Title: "Task not found", Status: http.StatusNotFound, Detail: "The requested task does not exist."}
}

func internalProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:internal", Title: "Internal server error", Status: http.StatusInternalServerError, Detail: "The server could not complete the request."}
}

func writeProblem(w http.ResponseWriter, value generated.Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(value.Status)
	_ = json.NewEncoder(w).Encode(value)
}
