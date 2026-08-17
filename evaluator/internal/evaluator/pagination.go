package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func (s *suite) seedPageTasks(ctx context.Context, count int, tied bool) ([]string, error) {
	ids := make([]string, count)
	for index := 0; index < count; index++ {
		ids[index] = fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1)
		createdAt := time.Date(2000, 1, 1, 0, 0, index, 0, time.UTC)
		if tied {
			createdAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		if _, err := s.db.Exec(ctx, "INSERT INTO tasks (id, title, created_at) VALUES ($1, $2, $3)", ids[index], fmt.Sprintf("task %03d", index+1), createdAt); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (s *suite) fetchPage(ctx context.Context, limit *int, cursor string) (taskList, error) {
	values := make(url.Values)
	if limit != nil {
		values.Set("limit", fmt.Sprint(*limit))
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	path := "/tasks"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	response, err := s.jsonRequest(ctx, http.MethodGet, path, "")
	if err != nil {
		return taskList{}, err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return taskList{}, err
	}
	return s.decodeTaskList(response.Body)
}

func pageCursor(page taskList) (string, bool, error) {
	if page.NextCursor == nil {
		return "", false, nil
	}
	if bytes.Equal(bytes.TrimSpace(page.NextCursor), []byte("null")) {
		return "", false, errors.New("nextCursor must be omitted, not null")
	}
	var cursor string
	if err := json.Unmarshal(page.NextCursor, &cursor); err != nil {
		return "", false, fmt.Errorf("nextCursor is not a string: %w", err)
	}
	if cursor == "" {
		return "", false, errors.New("nextCursor must not be empty")
	}
	return cursor, true, nil
}

func (s *suite) paginationDefaultLimit(ctx context.Context) error {
	ids, err := s.seedPageTasks(ctx, 21, false)
	if err != nil {
		return err
	}
	page, err := s.fetchPage(ctx, nil, "")
	if err != nil {
		return err
	}
	if err := checkTaskIDs(page.Items, ids[:20]); err != nil {
		return err
	}
	_, present, err := pageCursor(page)
	if err != nil || !present {
		return fmt.Errorf("default page must have nextCursor: %w", err)
	}
	return nil
}

func (s *suite) paginationLimitBounds(ctx context.Context) error {
	ids, err := s.seedPageTasks(ctx, 101, false)
	if err != nil {
		return err
	}
	one := 1
	page, err := s.fetchPage(ctx, &one, "")
	if err != nil {
		return err
	}
	if err := checkTaskIDs(page.Items, ids[:1]); err != nil {
		return fmt.Errorf("limit 1: %w", err)
	}
	if _, present, err := pageCursor(page); err != nil || !present {
		return fmt.Errorf("limit 1 cursor: %w", err)
	}
	hundred := 100
	page, err = s.fetchPage(ctx, &hundred, "")
	if err != nil {
		return err
	}
	if err := checkTaskIDs(page.Items, ids[:100]); err != nil {
		return fmt.Errorf("limit 100: %w", err)
	}
	if _, present, err := pageCursor(page); err != nil || !present {
		return fmt.Errorf("limit 100 cursor: %w", err)
	}
	return nil
}

func (s *suite) paginationInvalidLimit(ctx context.Context) error {
	for _, value := range []string{"0", "-1", "101", "text", "1.5"} {
		response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks?limit="+url.QueryEscape(value), "")
		if err != nil {
			return err
		}
		if err := expectProblemResponse(response, validationProblem()); err != nil {
			return fmt.Errorf("limit %q: %w", value, err)
		}
	}
	return nil
}

func (s *suite) paginationMalformedCursor(ctx context.Context) error {
	if _, err := s.seedPageTasks(ctx, 1, false); err != nil {
		return err
	}
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks?cursor=%25%25%25not-a-cursor", "")
	if err != nil {
		return err
	}
	return expectProblemResponse(response, validationProblem())
}

func (s *suite) paginationEmpty(ctx context.Context) error {
	page, err := s.fetchPage(ctx, nil, "")
	if err != nil {
		return err
	}
	if len(page.Items) != 0 {
		return fmt.Errorf("empty page has %d items", len(page.Items))
	}
	if _, present, err := pageCursor(page); err != nil || present {
		return fmt.Errorf("empty page cursor must be absent: %w", err)
	}
	return nil
}

func (s *suite) paginationSinglePage(ctx context.Context) error {
	ids, err := s.seedPageTasks(ctx, 3, false)
	if err != nil {
		return err
	}
	limit := 10
	page, err := s.fetchPage(ctx, &limit, "")
	if err != nil {
		return err
	}
	if err := checkTaskIDs(page.Items, ids); err != nil {
		return err
	}
	if _, present, err := pageCursor(page); err != nil || present {
		return fmt.Errorf("single page cursor must be absent: %w", err)
	}
	return nil
}

func (s *suite) paginationMultiplePages(ctx context.Context) error {
	ids, err := s.seedPageTasks(ctx, 11, false)
	if err != nil {
		return err
	}
	got, err := s.collectPages(ctx, 3)
	if err != nil {
		return err
	}
	return checkStringIDs(got, ids)
}

func (s *suite) paginationTimestampTie(ctx context.Context) error {
	ids, err := s.seedPageTasks(ctx, 7, true)
	if err != nil {
		return err
	}
	got, err := s.collectPages(ctx, 2)
	if err != nil {
		return err
	}
	return checkStringIDs(got, ids)
}

func (s *suite) paginationFinalPage(ctx context.Context) error {
	ids, err := s.seedPageTasks(ctx, 3, false)
	if err != nil {
		return err
	}
	limit := 2
	first, err := s.fetchPage(ctx, &limit, "")
	if err != nil {
		return err
	}
	cursor, present, err := pageCursor(first)
	if err != nil || !present {
		return fmt.Errorf("first page cursor: %w", err)
	}
	final, err := s.fetchPage(ctx, &limit, cursor)
	if err != nil {
		return err
	}
	if err := checkTaskIDs(final.Items, ids[2:]); err != nil {
		return err
	}
	if _, present, err := pageCursor(final); err != nil || present {
		return fmt.Errorf("final page cursor must be absent: %w", err)
	}
	return nil
}

func (s *suite) paginationCursorAfterDelete(ctx context.Context) error {
	ids, err := s.seedPageTasks(ctx, 4, false)
	if err != nil {
		return err
	}
	limit := 2
	first, err := s.fetchPage(ctx, &limit, "")
	if err != nil {
		return err
	}
	cursor, present, err := pageCursor(first)
	if err != nil || !present {
		return fmt.Errorf("first page cursor: %w", err)
	}
	if _, err := s.db.Exec(ctx, "DELETE FROM tasks WHERE id = $1 OR id = $2", ids[2], ids[3]); err != nil {
		return err
	}
	afterDelete, err := s.fetchPage(ctx, &limit, cursor)
	if err != nil {
		return err
	}
	if len(afterDelete.Items) != 0 {
		return fmt.Errorf("page after deleting later rows has ids %v, want empty", taskIDs(afterDelete.Items))
	}
	if _, present, err := pageCursor(afterDelete); err != nil || present {
		return fmt.Errorf("empty final page cursor must be absent: %w", err)
	}
	return nil
}

func (s *suite) collectPages(ctx context.Context, limit int) ([]string, error) {
	var result []string
	cursor := ""
	for pageNumber := 0; pageNumber < 100; pageNumber++ {
		page, err := s.fetchPage(ctx, &limit, cursor)
		if err != nil {
			return nil, err
		}
		if len(page.Items) > limit {
			return nil, fmt.Errorf("page has %d items, limit is %d", len(page.Items), limit)
		}
		for _, item := range page.Items {
			result = append(result, item.ID)
		}
		next, present, err := pageCursor(page)
		if err != nil {
			return nil, err
		}
		if !present {
			return result, nil
		}
		if next == cursor {
			return nil, errors.New("pagination cursor did not advance")
		}
		cursor = next
	}
	return nil, errors.New("pagination did not terminate within 100 pages")
}

func checkStringIDs(got, want []string) error {
	if len(got) != len(want) {
		return fmt.Errorf("got ids %v, want %v", got, want)
	}
	seen := make(map[string]bool, len(got))
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("got ids %v, want %v", got, want)
		}
		if seen[got[index]] {
			return fmt.Errorf("duplicate id %s", got[index])
		}
		seen[got[index]] = true
	}
	return nil
}
