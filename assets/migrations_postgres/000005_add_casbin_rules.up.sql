CREATE TABLE casbin_rules (
    id    BIGSERIAL PRIMARY KEY,
    ptype TEXT NOT NULL,
    v0    TEXT NOT NULL DEFAULT '',
    v1    TEXT NOT NULL DEFAULT '',
    v2    TEXT NOT NULL DEFAULT '',
    v3    TEXT NOT NULL DEFAULT '',
    v4    TEXT NOT NULL DEFAULT '',
    v5    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_casbin_ptype ON casbin_rules(ptype);
CREATE INDEX idx_casbin_v0_v1 ON casbin_rules(v0, v1);
