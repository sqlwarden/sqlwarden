
CREATE TABLE users (
    id SERIAL NOT NULL PRIMARY KEY,
    created TIMESTAMPTZ NOT NULL,
    email TEXT NOT NULL,
    hashed_password TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_users_lower_email ON users (LOWER(email));

