-- name: CreateTask :one
INSERT INTO tasks (id, title)
VALUES ($1, $2)
RETURNING id, title, created_at;

-- name: GetTask :one
SELECT id, title, created_at
FROM tasks
WHERE id = $1;

-- name: ListTasks :many
SELECT id, title, created_at
FROM tasks
ORDER BY created_at ASC, id ASC;

-- name: DeleteTask :one
DELETE FROM tasks
WHERE id = $1
RETURNING id;
