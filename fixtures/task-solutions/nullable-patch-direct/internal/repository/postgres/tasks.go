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
RETURNING id, title, created_at, due_at;`
	getTaskSQL = `SELECT id, title, created_at, due_at
FROM tasks
WHERE id = $1;`
	listTasksSQL = `SELECT id, title, created_at, due_at
FROM tasks
ORDER BY created_at ASC, id ASC;`
	deleteTaskSQL = `DELETE FROM tasks
WHERE id = $1
RETURNING id;`
	patchTaskSQL = `UPDATE tasks
SET title = CASE WHEN $1::boolean THEN $2::text ELSE title END,
    due_at = CASE WHEN $3::boolean THEN $4::timestamp with time zone ELSE due_at END
WHERE id = $5::uuid
RETURNING id, title, created_at, due_at;`
)

type TaskRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{pool: pool}
}

func (r *TaskRepository) Create(ctx context.Context, id, title string) (task.Task, error) {
	var result task.Task
	err := scanTask(r.pool.QueryRow(ctx, createTaskSQL, id, title), &result)
	if err != nil {
		return task.Task{}, fmt.Errorf("create task: %w", err)
	}
	return result, nil
}

func (r *TaskRepository) Get(ctx context.Context, id string) (task.Task, error) {
	var result task.Task
	err := scanTask(r.pool.QueryRow(ctx, getTaskSQL, id), &result)
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
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.DueAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return result, nil
}

func (r *TaskRepository) Patch(ctx context.Context, id string, patch task.Patch) (task.Task, error) {
	var title string
	if patch.Title != nil {
		title = *patch.Title
	}
	var result task.Task
	err := scanTask(r.pool.QueryRow(ctx, patchTaskSQL, patch.Title != nil, title, patch.DueAtPresent, patch.DueAt, id), &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("patch task: %w", err)
	}
	return result, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanTask(row rowScanner, result *task.Task) error {
	return row.Scan(&result.ID, &result.Title, &result.CreatedAt, &result.DueAt)
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
