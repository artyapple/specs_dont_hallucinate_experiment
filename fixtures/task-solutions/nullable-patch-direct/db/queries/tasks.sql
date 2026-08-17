-- name: CreateTask :one
INSERT INTO tasks (id, title)
VALUES ($1, $2)
RETURNING id, title, created_at, due_at;

-- name: GetTask :one
SELECT id, title, created_at, due_at
FROM tasks
WHERE id = $1;

-- name: ListTasks :many
SELECT id, title, created_at, due_at
FROM tasks
ORDER BY created_at ASC, id ASC;

-- name: DeleteTask :one
DELETE FROM tasks
WHERE id = $1
RETURNING id;

-- name: PatchTask :one
UPDATE tasks
SET title = CASE WHEN sqlc.arg(title_present)::boolean THEN sqlc.arg(title)::text ELSE title END,
    due_at = CASE WHEN sqlc.arg(due_at_present)::boolean THEN sqlc.narg(due_at)::timestamp with time zone ELSE due_at END
WHERE id = sqlc.arg(id)::uuid
RETURNING id, title, created_at, due_at;
