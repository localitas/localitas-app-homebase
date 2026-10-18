package homebase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// hapStore keeps the HomeKit bridge's pairing state in the app database
// instead of the container's filesystem, which is lost on every redeploy.
type hapStore struct {
	db *sql.DB
}

func (s *hapStore) Set(key string, value []byte) error {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO hap_store (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *hapStore) Get(key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRowContext(context.Background(), `SELECT value FROM hap_store WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("hap store: no value for %s", key)
	}
	return value, err
}

func (s *hapStore) Delete(key string) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM hap_store WHERE key = ?`, key)
	return err
}

func (s *hapStore) KeysWithSuffix(suffix string) ([]string, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT key FROM hap_store WHERE length(key) >= length(?) AND substr(key, length(key) - length(?) + 1) = ? ORDER BY key`, suffix, suffix, suffix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
