-- name: CreateTask :one
INSERT INTO tasks (id, title)
VALUES ($1, $2)
RETURNING id, title, created_at, version;

-- name: GetTask :one
SELECT id, title, created_at, version
FROM tasks
WHERE id = $1;

-- name: ListTasks :many
SELECT id, title, created_at, version
FROM tasks
ORDER BY created_at ASC, id ASC;

-- name: DeleteTask :one
DELETE FROM tasks
WHERE id = $1
RETURNING id;

-- name: UpdateTask :one
UPDATE tasks
SET title = $2, version = version + 1
WHERE id = $1 AND version = $3
RETURNING id, title, created_at, version;
