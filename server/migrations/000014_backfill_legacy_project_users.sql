-- Projects predate the users table and historically had no ownership foreign key.
INSERT INTO users (id)
SELECT DISTINCT p.user_id
FROM projects p
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = p.user_id);
