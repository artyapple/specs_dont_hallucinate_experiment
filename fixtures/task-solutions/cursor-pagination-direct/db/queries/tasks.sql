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
WHERE NOT $1::boolean
   OR (created_at, id) > ($2::timestamptz, $3::uuid)
ORDER BY created_at ASC, id ASC
LIMIT $4::integer;

-- name: DeleteTask :one
DELETE FROM tasks
WHERE id = $1
RETURNING id;
