package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specs-dont-hallucinate/taskservice/internal/task"
)

const (
	createTaskSQL = `INSERT INTO tasks (id, title)
VALUES ($1, $2)
RETURNING id, title, created_at, version;`
	getTaskSQL = `SELECT id, title, created_at, version
FROM tasks
WHERE id = $1;`
	listTasksSQL = `SELECT id, title, created_at, version
FROM tasks
ORDER BY created_at ASC, id ASC;`
	deleteTaskSQL = `DELETE FROM tasks
WHERE id = $1
RETURNING id;`
	updateTaskSQL = `UPDATE tasks
SET title = $2, version = version + 1
WHERE id = $1 AND version = $3
RETURNING id, title, created_at, version;`
)

type TaskRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{pool: pool}
}

func (r *TaskRepository) Create(ctx context.Context, id, title string) (task.Task, error) {
	var result task.Task
	err := r.pool.QueryRow(ctx, createTaskSQL, id, title).Scan(&result.ID, &result.Title, &result.CreatedAt, &result.Version)
	if err != nil {
		return task.Task{}, fmt.Errorf("create task: %w", err)
	}
	return result, nil
}

func (r *TaskRepository) Get(ctx context.Context, id string) (task.Task, error) {
	var result task.Task
	err := r.pool.QueryRow(ctx, getTaskSQL, id).Scan(&result.ID, &result.Title, &result.CreatedAt, &result.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("get task: %w", err)
	}
	return result, nil
}

func (r *TaskRepository) List(ctx context.Context) ([]task.Task, error) {
	rows, err := r.pool.Query(ctx, listTasksSQL)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	result := make([]task.Task, 0)
	for rows.Next() {
		var item task.Task
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.Version); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return result, nil
}

func (r *TaskRepository) Update(ctx context.Context, id, title string, expectedVersion int64) (task.Task, error) {
	var result task.Task
	err := r.pool.QueryRow(ctx, updateTaskSQL, id, title, expectedVersion).Scan(
		&result.ID, &result.Title, &result.CreatedAt, &result.Version,
	)
	if !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return task.Task{}, fmt.Errorf("update task: %w", err)
		}
		return result, nil
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM tasks WHERE id = $1)", id).Scan(&exists); err != nil {
		return task.Task{}, fmt.Errorf("check task after update: %w", err)
	}
	if !exists {
		return task.Task{}, task.ErrNotFound
	}
	return task.Task{}, task.ErrPreconditionFailed
}

func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	var deletedID string
	err := r.pool.QueryRow(ctx, deleteTaskSQL, id).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}
