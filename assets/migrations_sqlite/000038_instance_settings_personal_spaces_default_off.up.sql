-- Personal spaces has backend support but no finished frontend surface
-- yet; force the seeded instance row to the off default until that lands.
UPDATE instance_settings SET personal_spaces_enabled = FALSE WHERE id = 1;
