package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const nullableSetInput = "2030-01-02T03:04:05.123456+02:30"
const nullableSetUTC = "2030-01-02T00:34:05.123456Z"

func (s *suite) createNullable(ctx context.Context, title string) (task, error) {
	body, _ := json.Marshal(map[string]string{"title": title})
	response, err := s.jsonRequest(ctx, http.MethodPost, "/tasks", string(body))
	if err != nil {
		return task{}, err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusCreated, "application/json"); err != nil {
		return task{}, err
	}
	return s.decodeTask(response.Body)
}

func (s *suite) patchNullable(ctx context.Context, id, body string) (task, error) {
	response, err := s.jsonRequest(ctx, http.MethodPatch, "/tasks/"+id, body)
	if err != nil {
		return task{}, err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return task{}, err
	}
	return s.decodeTask(response.Body)
}

func dueAtValue(value task) (*string, error) {
	if value.DueAt == nil {
		return nil, errors.New("dueAt is missing")
	}
	if bytes.Equal(bytes.TrimSpace(value.DueAt), []byte("null")) {
		return nil, nil
	}
	var timestamp string
	if err := json.Unmarshal(value.DueAt, &timestamp); err != nil {
		return nil, fmt.Errorf("dueAt is not a string or null: %w", err)
	}
	if !isCanonicalTimestamp(timestamp) {
		return nil, fmt.Errorf("dueAt %q is not UTC with six fractional digits", timestamp)
	}
	return &timestamp, nil
}

func (s *suite) nullableInitialNull(ctx context.Context) error {
	created, err := s.createNullable(ctx, "new task")
	if err != nil {
		return err
	}
	dueAt, err := dueAtValue(created)
	if err != nil {
		return err
	}
	if dueAt != nil {
		return fmt.Errorf("new task dueAt = %v, want null", dueAt)
	}
	var stored *time.Time
	if err := s.db.QueryRow(ctx, "SELECT due_at FROM tasks WHERE id = $1", created.ID).Scan(&stored); err != nil {
		return err
	}
	if stored != nil {
		return fmt.Errorf("new task database due_at = %s, want null", stored.UTC())
	}

	const existingID = "00000000-0000-4000-8000-000000000001"
	if _, err := s.db.Exec(ctx, "INSERT INTO tasks (id, title) VALUES ($1, $2)", existingID, "existing"); err != nil {
		return fmt.Errorf("seed existing row: %w", err)
	}
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks/"+existingID, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return err
	}
	existing, err := s.decodeTask(response.Body)
	if err != nil {
		return err
	}
	dueAt, err = dueAtValue(existing)
	if err != nil {
		return err
	}
	if dueAt != nil {
		return fmt.Errorf("existing task dueAt = %v, want null", dueAt)
	}
	return nil
}

func (s *suite) nullablePostRejectsDueAt(ctx context.Context) error {
	for _, body := range []string{
		`{"title":"task","dueAt":null}`,
		`{"title":"task","dueAt":"2030-01-01T00:00:00Z"}`,
	} {
		response, err := s.jsonRequest(ctx, http.MethodPost, "/tasks", body)
		if err != nil {
			return err
		}
		if err := expectProblemResponse(response, validationProblem()); err != nil {
			return err
		}
	}
	return s.expectRowCount(ctx, 0)
}

func (s *suite) nullableOmittedPreserves(ctx context.Context) error {
	created, err := s.createNullable(ctx, "original")
	if err != nil {
		return err
	}
	set, err := s.patchNullable(ctx, created.ID, `{"dueAt":"`+nullableSetInput+`"}`)
	if err != nil {
		return err
	}
	updated, err := s.patchNullable(ctx, created.ID, `{"title":"changed"}`)
	if err != nil {
		return err
	}
	return compareNullableState(set, updated, "changed", nullableSetUTC)
}

func (s *suite) nullableNullClears(ctx context.Context) error {
	created, err := s.createNullable(ctx, "original")
	if err != nil {
		return err
	}
	if _, err := s.patchNullable(ctx, created.ID, `{"dueAt":"`+nullableSetInput+`"}`); err != nil {
		return err
	}
	cleared, err := s.patchNullable(ctx, created.ID, `{"dueAt":null}`)
	if err != nil {
		return err
	}
	dueAt, err := dueAtValue(cleared)
	if err != nil {
		return err
	}
	if dueAt != nil {
		return fmt.Errorf("cleared dueAt = %v, want null", dueAt)
	}
	if cleared.Title != created.Title || cleared.ID != created.ID || cleared.CreatedAt != created.CreatedAt {
		return errors.New("clearing dueAt changed immutable fields or title")
	}
	var stored *time.Time
	if err := s.db.QueryRow(ctx, "SELECT due_at FROM tasks WHERE id = $1", created.ID).Scan(&stored); err != nil {
		return err
	}
	if stored != nil {
		return errors.New("cleared dueAt remains non-null in database")
	}
	return nil
}

func (s *suite) nullableValueSets(ctx context.Context) error {
	created, err := s.createNullable(ctx, "original")
	if err != nil {
		return err
	}
	updated, err := s.patchNullable(ctx, created.ID, `{"dueAt":"`+nullableSetInput+`"}`)
	if err != nil {
		return err
	}
	return s.expectNullableStored(ctx, updated, "original", nullableSetUTC)
}

func (s *suite) nullableTitleOnly(ctx context.Context) error {
	created, err := s.createNullable(ctx, "original")
	if err != nil {
		return err
	}
	set, err := s.patchNullable(ctx, created.ID, `{"dueAt":"`+nullableSetInput+`"}`)
	if err != nil {
		return err
	}
	updated, err := s.patchNullable(ctx, created.ID, `{"title":"  世界 title  "}`)
	if err != nil {
		return err
	}
	if err := compareNullableState(set, updated, "世界 title", nullableSetUTC); err != nil {
		return err
	}
	return s.expectNullableStored(ctx, updated, "世界 title", nullableSetUTC)
}

func (s *suite) nullableBothFields(ctx context.Context) error {
	created, err := s.createNullable(ctx, "original")
	if err != nil {
		return err
	}
	updated, err := s.patchNullable(ctx, created.ID, `{"title":"  changed  ","dueAt":"`+nullableSetInput+`"}`)
	if err != nil {
		return err
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt {
		return errors.New("combined PATCH changed immutable fields")
	}
	return s.expectNullableStored(ctx, updated, "changed", nullableSetUTC)
}

func (s *suite) nullableEmptyRejected(ctx context.Context) error {
	return s.nullableRejectedWithoutMutation(ctx, `{}`)
}

func (s *suite) nullableUnknownFieldRejected(ctx context.Context) error {
	return s.nullableRejectedWithoutMutation(ctx, `{"title":"changed","extra":true}`)
}

func (s *suite) nullableTitleNullRejected(ctx context.Context) error {
	return s.nullableRejectedWithoutMutation(ctx, `{"title":null}`)
}

func (s *suite) nullableInvalidTitle(ctx context.Context) error {
	for _, body := range []string{
		`{"title":" \t\n "}`,
		`{"title":"` + strings.Repeat("界", 201) + `"}`,
	} {
		if err := s.nullableRejectedWithoutMutation(ctx, body); err != nil {
			return err
		}
		if err := s.reset(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *suite) nullableInvalidTimestamp(ctx context.Context) error {
	return s.nullableRejectedWithoutMutation(ctx, `{"dueAt":"not-a-timestamp"}`)
}

func (s *suite) nullableUnknownTask(ctx context.Context) error {
	response, err := s.jsonRequest(ctx, http.MethodPatch, "/tasks/ffffffff-ffff-4fff-8fff-ffffffffffff", `{"title":"changed"}`)
	if err != nil {
		return err
	}
	return expectProblemResponse(response, notFoundProblem())
}

func (s *suite) nullableGetConsistent(ctx context.Context) error {
	created, err := s.createNullable(ctx, "original")
	if err != nil {
		return err
	}
	patched, err := s.patchNullable(ctx, created.ID, `{"title":"  final  ","dueAt":"`+nullableSetInput+`"}`)
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
	if err := compareNullableState(patched, got, "final", nullableSetUTC); err != nil {
		return err
	}
	return s.expectNullableStored(ctx, got, "final", nullableSetUTC)
}

func (s *suite) nullableRejectedWithoutMutation(ctx context.Context, body string) error {
	created, err := s.createNullable(ctx, "original")
	if err != nil {
		return err
	}
	before, err := s.patchNullable(ctx, created.ID, `{"dueAt":"`+nullableSetInput+`"}`)
	if err != nil {
		return err
	}
	response, err := s.jsonRequest(ctx, http.MethodPatch, "/tasks/"+created.ID, body)
	if err != nil {
		return err
	}
	if err := expectProblemResponse(response, validationProblem()); err != nil {
		return err
	}
	getResponse, err := s.jsonRequest(ctx, http.MethodGet, "/tasks/"+created.ID, "")
	if err != nil {
		return err
	}
	defer getResponse.Body.Close()
	if err := expectStatusAndType(getResponse, http.StatusOK, "application/json"); err != nil {
		return err
	}
	after, err := s.decodeTask(getResponse.Body)
	if err != nil {
		return err
	}
	return compareNullableState(before, after, "original", nullableSetUTC)
}

func compareNullableState(before, after task, title, dueAt string) error {
	if after.ID != before.ID || after.CreatedAt != before.CreatedAt || after.Title != title {
		return fmt.Errorf("unexpected task state after PATCH: %+v", after)
	}
	actualDueAt, err := dueAtValue(after)
	if err != nil {
		return err
	}
	if actualDueAt == nil || *actualDueAt != dueAt {
		return fmt.Errorf("dueAt = %v, want %s", actualDueAt, dueAt)
	}
	return nil
}

func (s *suite) expectNullableStored(ctx context.Context, value task, title, dueAt string) error {
	actualDueAt, err := dueAtValue(value)
	if err != nil {
		return err
	}
	if value.Title != title || actualDueAt == nil || *actualDueAt != dueAt {
		return fmt.Errorf("response state = title %q dueAt %v, want %q %s", value.Title, actualDueAt, title, dueAt)
	}
	var storedTitle string
	var storedDueAt time.Time
	if err := s.db.QueryRow(ctx, "SELECT title, due_at FROM tasks WHERE id = $1", value.ID).Scan(&storedTitle, &storedDueAt); err != nil {
		return err
	}
	if storedTitle != title || storedDueAt.UTC().Format("2006-01-02T15:04:05.000000Z") != dueAt {
		return fmt.Errorf("database state = title %q dueAt %s, want %q %s", storedTitle, storedDueAt.UTC(), title, dueAt)
	}
	return nil
}

func (s *suite) expectRowCount(ctx context.Context, want int) error {
	var count int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM tasks").Scan(&count); err != nil {
		return err
	}
	if count != want {
		return fmt.Errorf("database row count = %d, want %d", count, want)
	}
	return nil
}
