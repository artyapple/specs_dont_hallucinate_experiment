package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
	DueAt     *time.Time
}

type Patch struct {
	Title        *string
	DueAtPresent bool
	DueAt        *time.Time
}

type Repository interface {
	Create(context.Context, string, string) (Task, error)
	Get(context.Context, string) (Task, error)
	List(context.Context) ([]Task, error)
	Delete(context.Context, string) error
	Patch(context.Context, string, Patch) (Task, error)
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

func (s *Service) List(ctx context.Context) ([]Task, error) {
	return s.repository.List(ctx)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if !validUUID(id) {
		return ErrInvalid
	}
	return s.repository.Delete(ctx, id)
}

func (s *Service) Patch(ctx context.Context, id string, patch Patch) (Task, error) {
	if !validUUID(id) || (patch.Title == nil && !patch.DueAtPresent) {
		return Task{}, ErrInvalid
	}
	if patch.Title != nil {
		title, err := normalizeTitle(*patch.Title)
		if err != nil {
			return Task{}, err
		}
		patch.Title = &title
	}
	return s.repository.Patch(ctx, id, patch)
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
