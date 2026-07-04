package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrUserExists is returned by CreateUser when the email is already registered.
var ErrUserExists = errors.New("user already exists")

// ErrUserNotFound is returned when a lookup finds no matching user.
var ErrUserNotFound = errors.New("user not found")

// User is the persisted account row.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// CreateUser inserts a new user with the given pre-hashed password. It returns
// ErrUserExists on a unique-constraint violation so the handler can map it to a
// generic conflict without leaking enumeration details.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES (?, ?)`, email, passwordHash)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrUserExists
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.GetUserByID(ctx, id)
}

// GetUserByEmail looks up a user by email, returning ErrUserNotFound if absent.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, created_at FROM users WHERE email = ?`, email))
}

// GetUserByID looks up a user by id, returning ErrUserNotFound if absent.
func (s *Store) GetUserByID(ctx context.Context, id int64) (User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, created_at FROM users WHERE id = ?`, id))
}

func (s *Store) scanUser(row *sql.Row) (User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	return u, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
// modernc.org/sqlite surfaces these in the error string, which we match without
// importing the driver's error type into this layer.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
