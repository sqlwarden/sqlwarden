CREATE UNIQUE INDEX workspaces_space_name_unique
    ON workspaces (owner_type, owner_id, name)
    WHERE owner_type = 'space';
