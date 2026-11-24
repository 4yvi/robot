// Package memory stores sessions, messages, summaries and user facts in SQLite.
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo
)

// Message is one stored dialog entry. Role is user, assistant, tool_call or tool.
type Message struct {
	Role     string
	Content  string
	ToolName string
	At       time.Time
}

// SessionInfo describes a stored session.
type SessionInfo struct {
	ID       int64
	Started  time.Time
	Ended    time.Time
	Summary  string
	Messages int
}

// Store is the persistence interface used by the session manager.
type Store interface {
	StartSession(ctx context.Context) (int64, error)
	AddMessage(ctx context.Context, session int64, m Message) error
	EndSession(ctx context.Context, session int64, summary string) error
	RecentSummaries(ctx context.Context, n int) ([]string, error)
	Facts(ctx context.Context) ([]string, error)
	AddFacts(ctx context.Context, facts []string) error
	Sessions(ctx context.Context, limit int) ([]SessionInfo, error)
	Messages(ctx context.Context, session int64) ([]Message, error)
	Search(ctx context.Context, query string, limit int) ([]string, error)
	Close() error
}

// SQLite implements Store.
type SQLite struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	summary TEXT NOT NULL DEFAULT '',
	summary_lc TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL REFERENCES sessions(id),
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	tool_name TEXT NOT NULL DEFAULT '',
	content_lc TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
CREATE TABLE IF NOT EXISTS facts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	fact TEXT NOT NULL UNIQUE,
	fact_lc TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);`

const timeFmt = time.RFC3339

// Open opens (and migrates) the database at path.
func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init database: %w", err)
	}
	return &SQLite{db: db}, nil
}

// Close closes the database.
func (s *SQLite) Close() error { return s.db.Close() }

func now() string { return time.Now().Format(timeFmt) }

// StartSession creates a session row.
func (s *SQLite) StartSession(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO sessions(started_at) VALUES(?)`, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddMessage appends a message to a session.
func (s *SQLite) AddMessage(ctx context.Context, session int64, m Message) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO messages(session_id, role, content, content_lc, tool_name, created_at) VALUES(?,?,?,?,?,?)`,
		session, m.Role, m.Content, strings.ToLower(m.Content), m.ToolName, now())
	return err
}

// EndSession marks the session finished and stores its summary.
func (s *SQLite) EndSession(ctx context.Context, session int64, summary string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET ended_at=?, summary=?, summary_lc=? WHERE id=?`, now(), summary, strings.ToLower(summary), session)
	return err
}

// RecentSummaries returns summaries of the n latest sessions that have one, oldest first.
func (s *SQLite) RecentSummaries(ctx context.Context, n int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT summary FROM sessions WHERE summary <> '' ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append([]string{v}, out...)
	}
	return out, rows.Err()
}

// Facts returns all stored facts about the user.
func (s *SQLite) Facts(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fact FROM facts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AddFacts stores new facts, ignoring duplicates.
func (s *SQLite) AddFacts(ctx context.Context, facts []string) error {
	for _, f := range facts {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO facts(fact, fact_lc, created_at) VALUES(?,?,?)`, f, strings.ToLower(f), now()); err != nil {
			return err
		}
	}
	return nil
}

// Sessions lists the latest sessions, newest first.
func (s *SQLite) Sessions(ctx context.Context, limit int) ([]SessionInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT s.id, s.started_at, COALESCE(s.ended_at,''), s.summary, (SELECT COUNT(*) FROM messages m WHERE m.session_id=s.id)
FROM sessions s ORDER BY s.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionInfo
	for rows.Next() {
		var si SessionInfo
		var st, en string
		if err := rows.Scan(&si.ID, &st, &en, &si.Summary, &si.Messages); err != nil {
			return nil, err
		}
		si.Started, _ = time.Parse(timeFmt, st)
		si.Ended, _ = time.Parse(timeFmt, en)
		out = append(out, si)
	}
	return out, rows.Err()
}

// Messages returns a session's messages in order.
func (s *SQLite) Messages(ctx context.Context, session int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT role, content, tool_name, created_at FROM messages WHERE session_id=? ORDER BY id`, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var at string
		if err := rows.Scan(&m.Role, &m.Content, &m.ToolName, &at); err != nil {
			return nil, err
		}
		m.At, _ = time.Parse(timeFmt, at)
		out = append(out, m)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Search finds past messages, summaries and facts containing every word of query.
func (s *SQLite) Search(ctx context.Context, query string, limit int) ([]string, error) {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil, nil
	}
	var out []string
	add := func(sqlText, label string, col string) error {
		var conds []string
		var args []any
		for _, w := range words {
			conds = append(conds, fmt.Sprintf("%s LIKE ? ESCAPE '\\'", col))
			args = append(args, "%"+escapeLike(w)+"%")
		}
		args = append(args, limit)
		rows, err := s.db.QueryContext(ctx, strings.Replace(sqlText, "{where}", strings.Join(conds, " AND "), 1), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a, b string
			if err := rows.Scan(&a, &b); err != nil {
				return err
			}
			out = append(out, fmt.Sprintf("[%s %s] %s", label, a, truncate(b, 300)))
		}
		return rows.Err()
	}
	if err := add(`SELECT substr(created_at,1,10), role || ': ' || content FROM messages WHERE role IN ('user','assistant') AND {where} ORDER BY id DESC LIMIT ?`, "диалог", "content_lc"); err != nil {
		return nil, err
	}
	if err := add(`SELECT substr(started_at,1,10), summary FROM sessions WHERE {where} ORDER BY id DESC LIMIT ?`, "сводка", "summary_lc"); err != nil {
		return nil, err
	}
	if err := add(`SELECT substr(created_at,1,10), fact FROM facts WHERE {where} ORDER BY id DESC LIMIT ?`, "факт", "fact_lc"); err != nil {
		return nil, err
	}
	return out, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
