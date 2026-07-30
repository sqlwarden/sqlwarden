UPDATE role_permissions
SET permission = 'conn:update'
WHERE permission = 'conn:write';
