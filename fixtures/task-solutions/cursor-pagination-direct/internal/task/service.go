package task

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid  = errors.New("invalid task input")
	ErrNotFound = errors.New("task not found")
)

type Task struct {
	ID        string
	Title     string
	CreatedAt time.Time
}

type PagePosition struct {
	CreatedAt time.Time
	ID        string
}

type Page struct {
	Items      []Task
	NextCursor string
}

type Repository interface {
	Create(context.Context, string, string) (Task, error)
	Get(context.Context, string) (Task, error)
	List(context.Context, int, *PagePosition) ([]Task, error)
	Delete(context.Context, string) error
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(ctx context.Context, title string) (Task, error) {
	title, err := normalizeTitle(title)
	if err != nil {
		return Task{}, err
	}
	id, err := newUUID()
	if err != nil {
		return Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	return s.repository.Create(ctx, id, title)
}

func (s *Service) Get(ctx context.Context, id string) (Task, error) {
	if !validUUID(id) {
		return Task{}, ErrInvalid
	}
	return s.repository.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, requestedLimit *int, encodedCursor *string) (Page, error) {
	limit := 20
	if requestedLimit != nil {
		limit = *requestedLimit
	}
	if limit < 1 || limit > 100 {
		return Page{}, ErrInvalid
	}

	var position *PagePosition
	if encodedCursor != nil {
		decoded, err := decodeCursor(*encodedCursor)
		if err != nil {
			return Page{}, ErrInvalid
		}
		position = &decoded
	}

	items, err := s.repository.List(ctx, limit+1, position)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor, err = encodeCursor(page.Items[len(page.Items)-1])
		if err != nil {
			return Page{}, fmt.Errorf("encode page cursor: %w", err)
		}
	}
	return page, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if !validUUID(id) {
		return ErrInvalid
	}
	return s.repository.Delete(ctx, id)
}

func normalizeTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	length := utf8.RuneCountInString(title)
	if length < 1 || length > 200 {
		return "", ErrInvalid
	}
	return title, nil
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}

type cursorPayload struct {
	CreatedAt string `json:"createdAt"`
	ID        string `json:"id"`
}

func encodeCursor(item Task) (string, error) {
	data, err := json.Marshal(cursorPayload{
		CreatedAt: item.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z"),
		ID:        item.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeCursor(encoded string) (PagePosition, error) {
	if encoded == "" {
		return PagePosition{}, ErrInvalid
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return PagePosition{}, ErrInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return PagePosition{}, ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return PagePosition{}, ErrInvalid
	}
	createdAt, err := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if err != nil || !validUUID(payload.ID) {
		return PagePosition{}, ErrInvalid
	}
	return PagePosition{CreatedAt: createdAt, ID: payload.ID}, nil
}
