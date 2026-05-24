ALTER TABLE instance_settings ADD COLUMN instance_name TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_settings ADD COLUMN instance_description TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_settings ADD COLUMN support_email TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_settings ADD COLUMN public_url TEXT NOT NULL DEFAULT '';
