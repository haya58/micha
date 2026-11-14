-- Prototype schema. One physical database per bot/persona.
-- No bot_id column by design: DATABASE_URL itself isolates bots.
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS messages(
  id BIGSERIAL PRIMARY KEY,
  guild_id TEXT NOT NULL,
  channel_id TEXT NOT NULL,
  thread_id TEXT NULL,
  user_id TEXT NOT NULL,
  username TEXT NOT NULL,
  content TEXT NOT NULL,
  attachments JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_messages_scope ON messages(guild_id, channel_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages(thread_id, created_at DESC) WHERE thread_id IS NOT NULL;

-- Discord message ID for idempotent polling of read-only channels.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS discord_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS uq_messages_discord ON messages(guild_id, discord_id);

CREATE TABLE IF NOT EXISTS memories(
  id BIGSERIAL PRIMARY KEY,
  guild_id TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  user_id TEXT NULL,
  kind TEXT NOT NULL DEFAULT 'fact',
  summary TEXT NOT NULL,
  embedding vector(1536),
  importance SMALLINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- HNSW index for cosine search. Scope filtering happens in WHERE clause.
CREATE INDEX IF NOT EXISTS idx_memories_embedding ON memories USING hnsw (embedding vector_cosine_ops);

CREATE TABLE IF NOT EXISTS tool_runs(
  id BIGSERIAL PRIMARY KEY,
  message_id BIGINT NULL REFERENCES messages(id) ON DELETE SET NULL,
  tool_name TEXT NOT NULL,
  args JSONB NOT NULL DEFAULT '{}',
  result TEXT NOT NULL DEFAULT '',
  latency_ms INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
