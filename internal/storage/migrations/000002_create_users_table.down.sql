-- Удаляем внешний ключ
ALTER TABLE urls DROP CONSTRAINT IF EXISTS fk_urls_user_id;

-- Удаляем индекс
DROP INDEX IF EXISTS idx_urls_user_id;

-- Удаляем таблицу users
DROP TABLE IF EXISTS users;

-- Удаляем колонку user_id
ALTER TABLE urls DROP COLUMN IF EXISTS user_id;