CREATE TABLE IF NOT EXISTS chats (
    id BIGSERIAL PRIMARY KEY
);

-- Сохраняем участников чата
CREATE TABLE IF NOT EXISTS chat_members (
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    PRIMARY KEY (chat_id, user_id)
);

-- Сохраняем сами сообщения
CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id BIGINT NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Индекс для быстрого получения истории сообщений
CREATE INDEX idx_messages_chat_id ON messages(chat_id);