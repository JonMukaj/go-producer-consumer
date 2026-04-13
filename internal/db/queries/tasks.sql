-- name: CreateTask :one
INSERT INTO tasks (type, value, state, creation_time, last_update_time)
VALUES ($1, $2, 'received', $3, $3)
RETURNING *;

-- name: UpdateTaskState :exec
UPDATE tasks
SET state = $2, last_update_time = $3
WHERE id = $1;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: CountByState :many
SELECT state, COUNT(*) AS count
FROM tasks
GROUP BY state;

-- name: CountByType :many
SELECT type, COUNT(*) AS count
FROM tasks
WHERE state = 'done'
GROUP BY type;

-- name: SumValueByType :many
SELECT type, COALESCE(SUM(value), 0)::BIGINT AS total
FROM tasks
WHERE state = 'done'
GROUP BY type;

-- name: CountByStateValue :one
SELECT COUNT(*) FROM tasks WHERE state = $1;
