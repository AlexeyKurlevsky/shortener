-- Удаляем внешний ключ с каскадным удалением зависимостей
ALTER TABLE urls DROP CONSTRAINT IF EXISTS fk_urls_user_id CASCADE;

-- Удаляем индекс
DROP INDEX IF EXISTS idx_urls_user_id;

-- Теперь можно удалить таблицу users
DROP TABLE IF EXISTS users CASCADE;

-- Удаляем колонку user_id
ALTER TABLE urls DROP COLUMN IF EXISTS user_id;