package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
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
	Update(context.Context, string, string, int64) (task.Task, error)
}

type Handler struct{ service Service }

func RegisterRoutes(router chi.Router, service Service) {
	adapter := &Handler{service: service}
	strict := generated.NewStrictHandlerWithOptions(adapter, nil, generated.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  func(w http.ResponseWriter, _ *http.Request, _ error) { writeProblem(w, validationProblem()) },
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) { writeProblem(w, internalProblem()) },
	})
	router.Group(func(router chi.Router) {
		router.Use(strictRequests)
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
	return generated.CreateTask201JSONResponse{
		Body:    response,
		Headers: generated.CreateTask201ResponseHeaders{ETag: etag(result.Version)},
	}, nil
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
	return generated.GetTask200JSONResponse{
		Body:    response,
		Headers: generated.GetTask200ResponseHeaders{ETag: etag(result.Version)},
	}, nil
}

func (h *Handler) UpdateTask(ctx context.Context, request generated.UpdateTaskRequestObject) (generated.UpdateTaskResponseObject, error) {
	if request.Body == nil {
		return updateValidationResponse(), nil
	}
	expectedVersion, _, ok := parseIfMatch([]string{request.Params.IfMatch})
	if !ok {
		return updateValidationResponse(), nil
	}
	result, err := h.service.Update(ctx, request.TaskId.String(), request.Body.Title, expectedVersion)
	if errors.Is(err, task.ErrInvalid) {
		return updateValidationResponse(), nil
	}
	if errors.Is(err, task.ErrNotFound) {
		return generated.UpdateTask404ApplicationProblemPlusJSONResponse{
			NotFoundProblemApplicationProblemPlusJSONResponse: generated.NotFoundProblemApplicationProblemPlusJSONResponse(notFoundProblem()),
		}, nil
	}
	if errors.Is(err, task.ErrPreconditionFailed) {
		return generated.UpdateTask412ApplicationProblemPlusJSONResponse{
			PreconditionFailedProblemApplicationProblemPlusJSONResponse: generated.PreconditionFailedProblemApplicationProblemPlusJSONResponse(preconditionFailedProblem()),
		}, nil
	}
	if err != nil {
		return generated.UpdateTask500ApplicationProblemPlusJSONResponse{
			InternalProblemApplicationProblemPlusJSONResponse: generated.InternalProblemApplicationProblemPlusJSONResponse(internalProblem()),
		}, nil
	}
	response, err := responseFromTask(result)
	if err != nil {
		return nil, err
	}
	return generated.UpdateTask200JSONResponse{
		Body:    response,
		Headers: generated.UpdateTask200ResponseHeaders{ETag: etag(result.Version)},
	}, nil
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

func strictRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var destination any
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/tasks":
			destination = &generated.CreateTaskRequest{}
		case r.Method == http.MethodPut:
			if _, status, ok := parseIfMatch(r.Header.Values("If-Match")); !ok {
				if status == http.StatusPreconditionRequired {
					writeProblem(w, preconditionRequiredProblem())
				} else {
					writeProblem(w, validationProblem())
				}
				return
			}
			destination = &generated.UpdateTaskRequest{}
		default:
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
		if err := decoder.Decode(destination); err != nil {
			writeProblem(w, validationProblem())
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeProblem(w, validationProblem())
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(contents))
		next.ServeHTTP(w, r)
	})
}

func parseIfMatch(values []string) (int64, int, bool) {
	if len(values) == 0 {
		return 0, http.StatusPreconditionRequired, false
	}
	if len(values) != 1 {
		return 0, http.StatusBadRequest, false
	}
	value := strings.Trim(values[0], " \t")
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, http.StatusBadRequest, false
	}
	digits := value[1 : len(value)-1]
	if digits[0] < '1' || digits[0] > '9' {
		return 0, http.StatusBadRequest, false
	}
	for index := 1; index < len(digits); index++ {
		if digits[index] < '0' || digits[index] > '9' {
			return 0, http.StatusBadRequest, false
		}
	}
	version, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, http.StatusBadRequest, false
	}
	return version, 0, true
}

func responseFromTask(value task.Task) (generated.Task, error) {
	id, err := uuid.Parse(value.ID)
	if err != nil {
		return generated.Task{}, err
	}
	return generated.Task{Id: id, Title: value.Title, CreatedAt: value.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z"), Version: value.Version}, nil
}

func createValidationResponse() generated.CreateTask400ApplicationProblemPlusJSONResponse {
	return generated.CreateTask400ApplicationProblemPlusJSONResponse{
		ValidationProblemApplicationProblemPlusJSONResponse: generated.ValidationProblemApplicationProblemPlusJSONResponse(validationProblem()),
	}
}

func validationProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:validation", Title: "Validation failed", Status: http.StatusBadRequest, Detail: "The request is invalid."}
}

func updateValidationResponse() generated.UpdateTask400ApplicationProblemPlusJSONResponse {
	return generated.UpdateTask400ApplicationProblemPlusJSONResponse{
		ValidationProblemApplicationProblemPlusJSONResponse: generated.ValidationProblemApplicationProblemPlusJSONResponse(validationProblem()),
	}
}

func notFoundProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:not-found", Title: "Task not found", Status: http.StatusNotFound, Detail: "The requested task does not exist."}
}

func internalProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:internal", Title: "Internal server error", Status: http.StatusInternalServerError, Detail: "The server could not complete the request."}
}

func preconditionRequiredProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:precondition-required", Title: "Precondition required", Status: http.StatusPreconditionRequired, Detail: "A valid If-Match header is required."}
}

func preconditionFailedProblem() generated.Problem {
	return generated.Problem{Type: "urn:problem:precondition-failed", Title: "Precondition failed", Status: http.StatusPreconditionFailed, Detail: "The supplied task version is stale."}
}

func etag(version int64) string {
	return strconv.Quote(strconv.FormatInt(version, 10))
}

func writeProblem(w http.ResponseWriter, value generated.Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(value.Status)
	_ = json.NewEncoder(w).Encode(value)
}
