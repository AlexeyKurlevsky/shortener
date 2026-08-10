-- Удаляем таблицу users вместе с зависимостями (внешним ключом)
DROP TABLE IF EXISTS users CASCADE;

-- Удаляем колонку user_id из таблицы urls
ALTER TABLE urls DROP COLUMN IF EXISTS user_id;

-- Удаляем индекс (если он остался)
DROP INDEX IF EXISTS idx_urls_user_id;