-- Удаляем индекс
DROP INDEX IF EXISTS idx_urls_user_id;

-- Удаляем колонку user_id
ALTER TABLE urls DROP COLUMN IF EXISTS user_id;

-- Удаляем таблицу users
DROP TABLE IF EXISTS users;