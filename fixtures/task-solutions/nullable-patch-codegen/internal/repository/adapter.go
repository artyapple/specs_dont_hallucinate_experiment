package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"specs-dont-hallucinate/taskservice/internal/repository/generated"
	"specs-dont-hallucinate/taskservice/internal/task"
)

type TaskRepository struct{ queries generated.Querier }

func NewTaskRepository(queries generated.Querier) *TaskRepository {
	return &TaskRepository{queries: queries}
}

func (r *TaskRepository) Create(ctx context.Context, id, title string) (task.Task, error) {
	result, err := r.queries.CreateTask(ctx, generated.CreateTaskParams{ID: id, Title: title})
	if err != nil {
		return task.Task{}, fmt.Errorf("create task: %w", err)
	}
	return mapTask(result.ID, result.Title, result.CreatedAt, result.DueAt), nil
}

func (r *TaskRepository) Get(ctx context.Context, id string) (task.Task, error) {
	result, err := r.queries.GetTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("get task: %w", err)
	}
	return mapTask(result.ID, result.Title, result.CreatedAt, result.DueAt), nil
}

func (r *TaskRepository) List(ctx context.Context) ([]task.Task, error) {
	items, err := r.queries.ListTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	result := make([]task.Task, len(items))
	for index, item := range items {
		result[index] = mapTask(item.ID, item.Title, item.CreatedAt, item.DueAt)
	}
	return result, nil
}

func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.queries.DeleteTask(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return task.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

func (r *TaskRepository) Patch(ctx context.Context, id string, patch task.Patch) (task.Task, error) {
	params := generated.PatchTaskParams{ID: id, TitlePresent: patch.Title != nil, DueAtPresent: patch.DueAtPresent}
	if patch.Title != nil {
		params.Title = *patch.Title
	}
	if patch.DueAt != nil {
		params.DueAt = pgtype.Timestamptz{Time: *patch.DueAt, Valid: true}
	}
	result, err := r.queries.PatchTask(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("patch task: %w", err)
	}
	return mapTask(result.ID, result.Title, result.CreatedAt, result.DueAt), nil
}

func mapTask(id, title string, createdAt time.Time, dueAt pgtype.Timestamptz) task.Task {
	result := task.Task{ID: id, Title: title, CreatedAt: createdAt}
	if dueAt.Valid {
		result.DueAt = &dueAt.Time
	}
	return result
}
