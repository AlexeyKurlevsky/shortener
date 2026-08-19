-- Добавляем колонку user_id (пока nullable)
ALTER TABLE urls ADD COLUMN IF NOT EXISTS user_id TEXT;

-- Создаём таблицу users
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Переносим существующие user_id из urls в users (если есть)
INSERT INTO users (id)
SELECT DISTINCT user_id
FROM urls
WHERE user_id IS NOT NULL
ON CONFLICT (id) DO NOTHING;

-- Индекс для быстрых запросов по пользователю (оставляем)
CREATE INDEX IF NOT EXISTS idx_urls_user_id ON urls(user_id);