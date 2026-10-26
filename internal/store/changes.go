package store

import (
	"context"
	"database/sql"
	"fmt"
)

// NewChange is a change to append to the log.
type NewChange struct {
	ID        string
	Kind      string
	RecordID  string
	Data      []byte
	UpdatedAt int64
	DeviceID  string
}

// StoredChange is a change read back from the log with its sequence number.
type StoredChange struct {
	Seq        int64
	ID         string
	Kind       string
	RecordID   string
	Data       []byte
	UpdatedAt  int64
	DeviceID   string
	ReceivedAt int64
}

// AppendChanges appends the changes in order and returns the sequence number of each.
// A change whose id is already stored keeps its original sequence number.
func (s *Store) AppendChanges(ctx context.Context, accountID int64, receivedAt int64, changes []NewChange) ([]int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin push: %w", err)
	}
	defer tx.Rollback()
	seqs := make([]int64, 0, len(changes))
	for _, change := range changes {
		var data any
		if change.Data != nil {
			data = string(change.Data)
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO changes (account_id, id, kind, record_id, data, updated_at, device_id, received_at)
			 SELECT ?, ?, ?, ?, ?, ?, ?, ?
			 WHERE NOT EXISTS (SELECT 1 FROM changes WHERE account_id = ? AND id = ?)
			 ON CONFLICT(account_id, id) DO NOTHING`,
			accountID, change.ID, change.Kind, change.RecordID, data, change.UpdatedAt, change.DeviceID, receivedAt,
			accountID, change.ID)
		if err != nil {
			return nil, fmt.Errorf("store change: %w", err)
		}
		var seq int64
		err = tx.QueryRowContext(ctx, `SELECT seq FROM changes WHERE account_id = ? AND id = ?`, accountID, change.ID).Scan(&seq)
		if err != nil {
			return nil, fmt.Errorf("read change seq: %w", err)
		}
		seqs = append(seqs, seq)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit push: %w", err)
	}
	return seqs, nil
}

// ChangesAfter returns up to limit changes with a sequence number above after,
// and whether more remain.
func (s *Store) ChangesAfter(ctx context.Context, accountID int64, after int64, limit int) ([]StoredChange, bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, id, kind, record_id, data, updated_at, device_id, received_at FROM changes
		 WHERE account_id = ? AND seq > ? ORDER BY seq LIMIT ?`, accountID, after, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("read changes: %w", err)
	}
	defer rows.Close()
	changes := []StoredChange{}
	for rows.Next() {
		var change StoredChange
		var data sql.NullString
		err := rows.Scan(&change.Seq, &change.ID, &change.Kind, &change.RecordID, &data,
			&change.UpdatedAt, &change.DeviceID, &change.ReceivedAt)
		if err != nil {
			return nil, false, fmt.Errorf("read change: %w", err)
		}
		if data.Valid {
			change.Data = []byte(data.String)
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read changes: %w", err)
	}
	more := len(changes) > limit
	if more {
		changes = changes[:limit]
	}
	return changes, more, nil
}
