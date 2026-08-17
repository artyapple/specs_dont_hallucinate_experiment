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
	return mapTask(result), nil
}

func (r *TaskRepository) Get(ctx context.Context, id string) (task.Task, error) {
	result, err := r.queries.GetTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("get task: %w", err)
	}
	return mapTask(result), nil
}

func (r *TaskRepository) List(ctx context.Context, pageSize int, position *task.PagePosition) ([]task.Task, error) {
	params := generated.ListTasksParams{
		Column1: position != nil,
		Column2: pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true},
		Column3: "00000000-0000-0000-0000-000000000000",
		Column4: int32(pageSize),
	}
	if position != nil {
		params.Column2.Time = position.CreatedAt
		params.Column3 = position.ID
	}
	items, err := r.queries.ListTasks(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	result := make([]task.Task, len(items))
	for index, item := range items {
		result[index] = mapTask(item)
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

func mapTask(value generated.Task) task.Task {
	return task.Task{ID: value.ID, Title: value.Title, CreatedAt: value.CreatedAt}
}
