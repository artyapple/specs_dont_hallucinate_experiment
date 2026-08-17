ALTER TABLE tasks
ADD COLUMN version bigint NOT NULL DEFAULT 1,
ADD CONSTRAINT tasks_version_positive CHECK (version > 0);
