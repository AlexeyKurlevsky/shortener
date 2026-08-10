-- Явно удаляем внешний ключ перед удалением таблицы users
ALTER TABLE urls DROP CONSTRAINT IF EXISTS fk_urls_user_id;

-- Теперь можно безопасно удалить таблицу users
DROP TABLE IF EXISTS users;

-- Удаляем колонку user_id
ALTER TABLE urls DROP COLUMN IF EXISTS user_id;

-- Удаляем индекс (если он остался)
DROP INDEX IF EXISTS idx_urls_user_id;