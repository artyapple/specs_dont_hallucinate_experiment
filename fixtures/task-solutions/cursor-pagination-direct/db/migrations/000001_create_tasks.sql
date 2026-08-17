CREATE TABLE tasks (
    id uuid PRIMARY KEY,
    title text NOT NULL,
    created_at timestamp(6) with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT tasks_title_length CHECK (char_length(title) BETWEEN 1 AND 200)
);

CREATE INDEX tasks_created_at_id_idx ON tasks (created_at ASC, id ASC);
