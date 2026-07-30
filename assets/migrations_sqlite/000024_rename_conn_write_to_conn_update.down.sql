UPDATE role_permissions
SET permission = 'conn:write'
WHERE permission = 'conn:update';
