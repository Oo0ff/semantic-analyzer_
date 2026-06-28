package auth

import (
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/mattn/go-sqlite3"
)

// User представляет пользователя системы.
type User struct {
	ID           int    `json:"id"`
	Login        string `json:"login"`
	Name         string `json:"name"`
	Organization string `json:"organization"` // новое поле
}

// Store – хранилище пользователей в SQLite.
type Store struct {
	db *sql.DB
}

// NewStore создаёт новое хранилище, инициализируя таблицу и миграции.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open auth db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := createTable(db); err != nil {
		return nil, err
	}
	// Миграция – добавление поля organization, если его ещё нет
	addColumnIfNotExists(db, "organization", "TEXT DEFAULT ''")
	return &Store{db: db}, nil
}

// createTable создаёт таблицу users, если она отсутствует.
func createTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		login TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		organization TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

// addColumnIfNotExists добавляет колонку, если она ещё не существует.
func addColumnIfNotExists(db *sql.DB, colName, colDef string) {
	rows, err := db.Query("PRAGMA table_info(users)")
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
			return // колонка уже есть
		}
	}
	// Выполняем ALTER TABLE
	db.Exec(fmt.Sprintf("ALTER TABLE users ADD COLUMN %s %s", colName, colDef))
}

// CreateUser регистрирует нового пользователя.
func (s *Store) CreateUser(login, password, name, organization string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO users (login, password_hash, name, organization) VALUES (?, ?, ?, ?)",
		login, string(hashed), name, organization)
	return err
}

// Authenticate проверяет логин и пароль, возвращает пользователя.
func (s *Store) Authenticate(login, password string) (*User, error) {
	var user User
	var hash string
	// organization может отсутствовать в старых записях, используем COALESCE
	err := s.db.QueryRow(`
		SELECT id, login, password_hash, name, COALESCE(organization, '') 
		FROM users WHERE login = ?`, login).
		Scan(&user.ID, &user.Login, &hash, &user.Name, &user.Organization)
	if err != nil {
		return nil, fmt.Errorf("неверный логин или пароль")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, fmt.Errorf("неверный логин или пароль")
	}
	return &user, nil
}

// Close закрывает соединение с БД.
func (s *Store) Close() error {
	return s.db.Close()
}