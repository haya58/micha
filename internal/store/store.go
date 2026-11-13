package store

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	Pool *pgxpool.Pool
	Dim  int
}

func Connect(ctx context.Context, databaseURL string, dim int) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{Pool: pool, Dim: dim}
	if err := s.Migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() { s.Pool.Close() }

// Migrate applies the embedded schema. Keeps it idempotent for prototype use.
func (s *Store) Migrate(ctx context.Context) error {
	sql := strings.ReplaceAll(schemaSQL, "vector(1536)", fmt.Sprintf("vector(%d)", s.Dim))
	_, err := s.Pool.Exec(ctx, sql)
	return err
}

type Message struct {
	ID          int64
	GuildID     string
	ChannelID   string
	ThreadID    *string
	UserID      string
	Username    string
	Content     string
	Attachments string
}

func (s *Store) SaveMessage(ctx context.Context, m Message) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO messages(guild_id, channel_id, thread_id, user_id, username, content, attachments)
		VALUES($1,$2,$3,$4,$5,$6,COALESCE($7,'[]')::jsonb) RETURNING id`,
		m.GuildID, m.ChannelID, m.ThreadID, m.UserID, m.Username, m.Content, m.Attachments).Scan(&id)
	return id, err
}

// RecentByScope returns the latest messages for a channel or thread scope.
func (s *Store) RecentByScope(ctx context.Context, guildID, channelID string, threadID *string, limit int) ([]Message, error) {
	var rows []Message
	var err error
	if threadID != nil && *threadID != "" {
		r, e := s.Pool.Query(ctx, `
			SELECT id, guild_id, channel_id, thread_id, user_id, username, content
			FROM messages WHERE guild_id=$1 AND thread_id=$2 ORDER BY id DESC LIMIT $3`,
			guildID, *threadID, limit)
		if e != nil {
			return nil, e
		}
		defer r.Close()
		for r.Next() {
			var m Message
			if e := r.Scan(&m.ID, &m.GuildID, &m.ChannelID, &m.ThreadID, &m.UserID, &m.Username, &m.Content); e != nil {
				return nil, e
			}
			rows = append(rows, m)
		}
		err = r.Err()
	} else {
		r, e := s.Pool.Query(ctx, `
			SELECT id, guild_id, channel_id, thread_id, user_id, username, content
			FROM messages WHERE guild_id=$1 AND channel_id=$2 AND thread_id IS NULL
			ORDER BY id DESC LIMIT $3`,
			guildID, channelID, limit)
		if e != nil {
			return nil, e
		}
		defer r.Close()
		for r.Next() {
			var m Message
			if e := r.Scan(&m.ID, &m.GuildID, &m.ChannelID, &m.ThreadID, &m.UserID, &m.Username, &m.Content); e != nil {
				return nil, e
			}
			rows = append(rows, m)
		}
		err = r.Err()
	}
	// reverse to chronological order
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, err
}

type Memory struct {
	ID         int64
	ScopeKey   string
	Kind       string
	Summary    string
	Importance int
}

func vecLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}

// SaveMemory stores a long-term memory with embedding.
func (s *Store) SaveMemory(ctx context.Context, guildID, scopeKey, kind, summary string, emb []float32, importance int) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO memories(guild_id, scope_key, kind, summary, embedding, importance)
		VALUES($1,$2,$3,$4,$5::vector,$6)`,
		guildID, scopeKey, kind, summary, vecLiteral(emb), importance)
	return err
}

// SearchMemories returns top-K memories by cosine similarity within the guild.
func (s *Store) SearchMemories(ctx context.Context, guildID string, emb []float32, topK int) ([]Memory, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, scope_key, kind, summary, importance
		FROM memories WHERE guild_id=$1 AND embedding IS NOT NULL
		ORDER BY embedding <=> $2::vector LIMIT $3`,
		guildID, vecLiteral(emb), topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.ScopeKey, &m.Kind, &m.Summary, &m.Importance); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SaveToolRun(ctx context.Context, messageID *int64, tool, args, result string, latencyMs int) error {
	if len(result) > 8000 {
		result = result[:8000]
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO tool_runs(message_id, tool_name, args, result, latency_ms)
		VALUES($1,$2,$3::jsonb,$4,$5)`,
		messageID, tool, args, result, latencyMs)
	return err
}
