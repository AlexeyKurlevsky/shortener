-- 1. Удаляем внешний ключ, чтобы разорвать зависимость
ALTER TABLE urls DROP CONSTRAINT IF EXISTS fk_urls_user_id;

-- 2. Удаляем индекс (если он ещё существует)
DROP INDEX IF EXISTS idx_urls_user_id;

-- 3. Удаляем колонку user_id (она больше не нужна)
ALTER TABLE urls DROP COLUMN IF EXISTS user_id;

-- 4. Теперь таблица users не имеет зависимостей — удаляем её
DROP TABLE IF EXISTS users;