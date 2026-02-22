-- name: InsertUser :one
INSERT INTO users (created, email, hashed_password)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE LOWER(email) = LOWER($1);

-- name: UpdateUserHashedPassword :exec
UPDATE users SET hashed_password = $1 WHERE id = $2;
