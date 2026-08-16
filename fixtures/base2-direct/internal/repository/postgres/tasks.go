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
RETURNING id, title, created_at;`
	getTaskSQL = `SELECT id, title, created_at
FROM tasks
WHERE id = $1;`
	listTasksSQL = `SELECT id, title, created_at
FROM tasks
ORDER BY created_at ASC, id ASC;`
	deleteTaskSQL = `DELETE FROM tasks
WHERE id = $1
RETURNING id;`
)

type TaskRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{pool: pool}
}

func (r *TaskRepository) Create(ctx context.Context, id, title string) (task.Task, error) {
	var result task.Task
	err := r.pool.QueryRow(ctx, createTaskSQL, id, title).Scan(&result.ID, &result.Title, &result.CreatedAt)
	if err != nil {
		return task.Task{}, fmt.Errorf("create task: %w", err)
	}
	return result, nil
}

func (r *TaskRepository) Get(ctx context.Context, id string) (task.Task, error) {
	var result task.Task
	err := r.pool.QueryRow(ctx, getTaskSQL, id).Scan(&result.ID, &result.Title, &result.CreatedAt)
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
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return result, nil
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
