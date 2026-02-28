
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    email TEXT NOT NULL,
    hashed_password TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_users_lower_email ON users (LOWER(email));


