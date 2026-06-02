ALTER TABLE role_bindings
    DROP CONSTRAINT IF EXISTS role_bindings_role_id_fkey;

ALTER TABLE role_bindings
    ADD CONSTRAINT role_bindings_role_id_fkey
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE RESTRICT;
