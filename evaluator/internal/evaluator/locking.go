package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

func (s *suite) createLocking(ctx context.Context, title string) (task, string, error) {
	body, _ := json.Marshal(map[string]string{"title": title})
	response, err := s.jsonRequest(ctx, http.MethodPost, "/tasks", string(body))
	if err != nil {
		return task{}, "", err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusCreated, "application/json"); err != nil {
		return task{}, "", err
	}
	created, err := s.decodeTask(response.Body)
	if err != nil {
		return task{}, "", err
	}
	return created, response.Header.Get("ETag"), nil
}

func (s *suite) putLocking(ctx context.Context, id, etag, body string) (task, string, error) {
	headers := make(http.Header)
	if etag != "" {
		headers.Set("If-Match", etag)
	}
	response, err := s.jsonRequestWithHeaders(ctx, http.MethodPut, "/tasks/"+id, body, headers)
	if err != nil {
		return task{}, "", err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return task{}, "", err
	}
	updated, err := s.decodeTask(response.Body)
	if err != nil {
		return task{}, "", err
	}
	return updated, response.Header.Get("ETag"), nil
}

func (s *suite) lockingInitialVersion(ctx context.Context) error {
	created, etag, err := s.createLocking(ctx, "task")
	if err != nil {
		return err
	}
	if created.Version == nil || *created.Version != 1 || etag != `"1"` {
		return fmt.Errorf("initial version/ETag = %v/%q, want 1/%q", created.Version, etag, `"1"`)
	}
	var stored int64
	if err := s.db.QueryRow(ctx, "SELECT version FROM tasks WHERE id = $1", created.ID).Scan(&stored); err != nil {
		return err
	}
	if stored != 1 {
		return fmt.Errorf("database version = %d, want 1", stored)
	}
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks", "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return err
	}
	list, err := s.decodeTaskList(response.Body)
	if err != nil {
		return err
	}
	if len(list.Items) != 1 || list.Items[0].Version == nil || *list.Items[0].Version != 1 {
		return errors.New("collection does not expose initial integer version 1")
	}
	return nil
}

func (s *suite) lockingGetETag(ctx context.Context) error {
	created, _, err := s.createLocking(ctx, "task")
	if err != nil {
		return err
	}
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks/"+created.ID, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return err
	}
	got, err := s.decodeTask(response.Body)
	if err != nil {
		return err
	}
	if got.Version == nil || response.Header.Get("ETag") != fmt.Sprintf(`"%d"`, *got.Version) {
		return fmt.Errorf("GET ETag %q does not match version %v", response.Header.Get("ETag"), got.Version)
	}
	return nil
}

func (s *suite) lockingPutSuccess(ctx context.Context) error {
	created, etag, err := s.createLocking(ctx, "original")
	if err != nil {
		return err
	}
	updated, newETag, err := s.putLocking(ctx, created.ID, etag, `{"title":"  世界 title  "}`)
	if err != nil {
		return err
	}
	if updated.Title != "世界 title" || updated.Version == nil || *updated.Version != 2 || newETag != `"2"` {
		return fmt.Errorf("updated task/ETag = %+v/%q, want trimmed title, version 2, ETag %q", updated, newETag, `"2"`)
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt {
		return errors.New("PUT changed immutable fields")
	}
	return s.expectLockingStored(ctx, updated.ID, "世界 title", 2)
}

func (s *suite) lockingMissingIfMatch(ctx context.Context) error {
	response, err := s.jsonRequest(ctx, http.MethodPut, "/tasks/ffffffff-ffff-4fff-8fff-ffffffffffff", `{"title":"changed"}`)
	if err != nil {
		return err
	}
	return expectProblemResponse(response, preconditionRequiredProblem())
}

func (s *suite) lockingMalformedIfMatch(ctx context.Context) error {
	values := [][]string{
		{`"0"`}, {`"+1"`}, {`"-1"`}, {`"01"`}, {`"9223372036854775808"`},
		{"1"}, {`W/"1"`}, {"*"}, {`"1", "2"`}, {`"1"`, `"2"`},
	}
	for _, headerValues := range values {
		headers := make(http.Header)
		for _, value := range headerValues {
			headers.Add("If-Match", value)
		}
		response, err := s.jsonRequestWithHeaders(ctx, http.MethodPut, "/tasks/ffffffff-ffff-4fff-8fff-ffffffffffff", `{"title":"changed"}`, headers)
		if err != nil {
			return err
		}
		if err := expectProblemResponse(response, validationProblem()); err != nil {
			return fmt.Errorf("If-Match %q: %w", headerValues, err)
		}
	}
	return nil
}

func (s *suite) lockingStaleIfMatch(ctx context.Context) error {
	created, etag, err := s.createLocking(ctx, "original")
	if err != nil {
		return err
	}
	if _, _, err := s.putLocking(ctx, created.ID, etag, `{"title":"winner"}`); err != nil {
		return err
	}
	headers := http.Header{"If-Match": []string{etag}}
	response, err := s.jsonRequestWithHeaders(ctx, http.MethodPut, "/tasks/"+created.ID, `{"title":"stale"}`, headers)
	if err != nil {
		return err
	}
	if err := expectProblemResponse(response, preconditionFailedProblem()); err != nil {
		return err
	}
	return s.expectLockingStored(ctx, created.ID, "winner", 2)
}

func (s *suite) lockingUnknownTask(ctx context.Context) error {
	headers := http.Header{"If-Match": []string{`"1"`}}
	response, err := s.jsonRequestWithHeaders(ctx, http.MethodPut, "/tasks/ffffffff-ffff-4fff-8fff-ffffffffffff", `{"title":"changed"}`, headers)
	if err != nil {
		return err
	}
	return expectProblemResponse(response, notFoundProblem())
}

func (s *suite) lockingUnknownField(ctx context.Context) error {
	return s.lockingRejectedWithoutMutation(ctx, `{"title":"changed","extra":true}`)
}

func (s *suite) lockingInvalidTitle(ctx context.Context) error {
	for _, body := range []string{
		`{"title":" \t\n "}`,
		`{"title":"` + strings.Repeat("界", 201) + `"}`,
	} {
		if err := s.lockingRejectedWithoutMutation(ctx, body); err != nil {
			return err
		}
		if err := s.reset(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *suite) lockingConcurrentSingleWinner(ctx context.Context) error {
	created, etag, err := s.createLocking(ctx, "original")
	if err != nil {
		return err
	}
	titles := []string{"first", "second"}
	type outcome struct {
		status      int
		body        []byte
		etag        string
		contentType string
		err         error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, title := range titles {
		go func(title string) {
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodPut, s.baseURL+"/tasks/"+created.ID, strings.NewReader(`{"title":"`+title+`"}`))
			if requestErr != nil {
				outcomes <- outcome{err: requestErr}
				return
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("If-Match", etag)
			ready.Done()
			<-start
			response, requestErr := s.client.Do(request)
			if requestErr != nil {
				outcomes <- outcome{err: requestErr}
				return
			}
			body, readErr := readAndClose(response)
			outcomes <- outcome{
				status:      response.StatusCode,
				body:        body,
				etag:        response.Header.Get("ETag"),
				contentType: response.Header.Get("Content-Type"),
				err:         readErr,
			}
		}(title)
	}
	ready.Wait()
	close(start)
	results := []outcome{<-outcomes, <-outcomes}
	winnerTitle := ""
	successes, stale := 0, 0
	for _, result := range results {
		if result.err != nil {
			return result.err
		}
		switch result.status {
		case http.StatusOK:
			successes++
			if result.etag != `"2"` {
				return fmt.Errorf("winner ETag = %q, want %q", result.etag, `"2"`)
			}
			winner, err := decodeTaskFor(strings.NewReader(string(result.body)), TaskLocking)
			if err != nil {
				return err
			}
			if winner.Version == nil || *winner.Version != 2 {
				return errors.New("winning response does not have version 2")
			}
			winnerTitle = winner.Title
		case http.StatusPreconditionFailed:
			stale++
			response := &http.Response{StatusCode: result.status, Header: make(http.Header), Body: ioNopCloser(result.body)}
			response.Header.Set("Content-Type", result.contentType)
			if err := expectProblemResponse(response, preconditionFailedProblem()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("concurrent PUT status = %d, want 200 or 412; body=%s", result.status, result.body)
		}
	}
	if successes != 1 || stale != 1 || (winnerTitle != "first" && winnerTitle != "second") {
		return fmt.Errorf("concurrent outcomes: successes=%d stale=%d winner=%q", successes, stale, winnerTitle)
	}
	return s.expectLockingStored(ctx, created.ID, winnerTitle, 2)
}

func (s *suite) lockingRejectedWithoutMutation(ctx context.Context, body string) error {
	created, etag, err := s.createLocking(ctx, "original")
	if err != nil {
		return err
	}
	headers := http.Header{"If-Match": []string{etag}}
	response, err := s.jsonRequestWithHeaders(ctx, http.MethodPut, "/tasks/"+created.ID, body, headers)
	if err != nil {
		return err
	}
	if err := expectProblemResponse(response, validationProblem()); err != nil {
		return err
	}
	return s.expectLockingStored(ctx, created.ID, "original", 1)
}

func (s *suite) expectLockingStored(ctx context.Context, id, title string, version int64) error {
	var storedTitle string
	var storedVersion int64
	if err := s.db.QueryRow(ctx, "SELECT title, version FROM tasks WHERE id = $1", id).Scan(&storedTitle, &storedVersion); err != nil {
		return err
	}
	if storedTitle != title || storedVersion != version {
		return fmt.Errorf("database title/version = %q/%d, want %q/%d", storedTitle, storedVersion, title, version)
	}
	return nil
}

func readAndClose(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	return io.ReadAll(io.LimitReader(response.Body, responseBodyDecodeLimit))
}

func ioNopCloser(body []byte) io.ReadCloser {
	return io.NopCloser(strings.NewReader(string(body)))
}
