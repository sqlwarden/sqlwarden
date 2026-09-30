ALTER TABLE instance_settings DROP COLUMN schema_lazy_threshold;
DELETE FROM jobs WHERE type = 'schema_sync';
