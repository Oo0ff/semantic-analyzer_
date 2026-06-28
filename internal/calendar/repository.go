package calendar

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Event – модель события (расширена полями ID, Completed)
type Event struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description,omitempty"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	AllDay       bool      `json:"all_day"`
	Location     string    `json:"location,omitempty"`
	Attendees    []string  `json:"attendees,omitempty"`
	Assignee     string    `json:"assignee,omitempty"`
	TranscriptID string    `json:"transcript_id,omitempty"`
	SourceText   string    `json:"source_text,omitempty"`
	Completed    bool      `json:"completed"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB: %w", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT,
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		all_day INTEGER DEFAULT 0,
		location TEXT,
		attendees TEXT,
		assignee TEXT DEFAULT '',
		transcript_id TEXT DEFAULT '',
		source_text TEXT,
		completed INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return nil, fmt.Errorf("failed to create events table: %w", err)
	}
	addColumnIfNotExists(db, "assignee", "TEXT DEFAULT ''")
	addColumnIfNotExists(db, "transcript_id", "TEXT DEFAULT ''")
	addColumnIfNotExists(db, "completed", "INTEGER DEFAULT 0")
	return &Repository{db: db}, nil
}

func addColumnIfNotExists(db *sql.DB, colName, colDef string) {
	rows, err := db.Query("PRAGMA table_info(events)")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == colName {
			return
		}
	}
	db.Exec(fmt.Sprintf("ALTER TABLE events ADD COLUMN %s %s", colName, colDef))
}

func (r *Repository) Insert(ev Event) (int64, error) {
	result, err := r.db.Exec(
		"INSERT INTO events (title, description, start_time, end_time, all_day, location, attendees, assignee, transcript_id, source_text, completed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		ev.Title, ev.Description, ev.Start, ev.End, boolToInt(ev.AllDay), ev.Location, stringsJoin(ev.Attendees, ", "), ev.Assignee, ev.TranscriptID, ev.SourceText, boolToInt(ev.Completed),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// DeleteByTranscriptID удаляет все события, связанные с переданным идентификатором транскрипта
func (r *Repository) DeleteByTranscriptID(transcriptID string) error {
	_, err := r.db.Exec("DELETE FROM events WHERE transcript_id = ?", transcriptID)
	return err
}

// DeleteAll удаляет все события из таблицы
func (r *Repository) DeleteAll() error {
	_, err := r.db.Exec("DELETE FROM events")
	return err
}

func (r *Repository) List(from, to time.Time) ([]Event, error) {
	query := "SELECT id, title, description, start_time, end_time, all_day, location, attendees, assignee, source_text, completed FROM events"
	var rows *sql.Rows
	var err error
	if !from.IsZero() && !to.IsZero() {
		query += " WHERE start_time BETWEEN ? AND ?"
		rows, err = r.db.Query(query, from, to)
	} else {
		rows, err = r.db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		var ev Event
		var allDay int
		var completed int
		var attendees, assignee sql.NullString
		err = rows.Scan(&ev.ID, &ev.Title, &ev.Description, &ev.Start, &ev.End, &allDay, &ev.Location, &attendees, &assignee, &ev.SourceText, &completed)
		if err != nil {
			return nil, err
		}
		ev.AllDay = allDay != 0
		ev.Completed = completed != 0
		if attendees.Valid {
			ev.Attendees = strings.Split(attendees.String, ", ")
		}
		if assignee.Valid {
			ev.Assignee = assignee.String
		}
		events = append(events, ev)
	}
	return events, nil
}

// MarkCompleted устанавливает флаг completed для события с указанным id.
func (r *Repository) MarkCompleted(id int, completed bool) error {
	_, err := r.db.Exec("UPDATE events SET completed = ? WHERE id = ?", boolToInt(completed), id)
	return err
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func stringsJoin(a []string, sep string) string {
	return strings.Join(a, sep)
}