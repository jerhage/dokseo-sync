package store

import (
	"context"
	"database/sql"
	"fmt"
)

type Device struct {
	ID         string
	AccountID  int64
	Name       string
	CreatedAt  int64
	LastSeenAt *int64
}

type NewDevice struct {
	ID        string
	Name      string
	TokenHash string
	CreatedAt int64
}

func (s *Store) CreateDevice(ctx context.Context, accountID int64, device NewDevice) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO devices (id, account_id, name, token_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		device.ID, accountID, device.Name, device.TokenHash, device.CreatedAt)
	if err != nil {
		return fmt.Errorf("create device: %w", err)
	}
	return nil
}

func (s *Store) DeviceByTokenHash(ctx context.Context, tokenHash string) (Device, bool, error) {
	var device Device
	err := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, created_at, last_seen_at FROM devices
		 WHERE token_hash = ? AND revoked_at IS NULL`, tokenHash).
		Scan(&device.ID, &device.AccountID, &device.Name, &device.CreatedAt, &device.LastSeenAt)
	if err == sql.ErrNoRows {
		return Device{}, false, nil
	}
	if err != nil {
		return Device{}, false, fmt.Errorf("find device by token: %w", err)
	}
	return device, true, nil
}

func (s *Store) TouchDevice(ctx context.Context, accountID int64, deviceID string, seenAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET last_seen_at = ? WHERE account_id = ? AND id = ?`, seenAt, accountID, deviceID)
	if err != nil {
		return fmt.Errorf("touch device: %w", err)
	}
	return nil
}

func (s *Store) ListDevices(ctx context.Context, accountID int64) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, account_id, name, created_at, last_seen_at FROM devices
		 WHERE account_id = ? AND revoked_at IS NULL ORDER BY created_at, id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		var device Device
		if err := rows.Scan(&device.ID, &device.AccountID, &device.Name, &device.CreatedAt, &device.LastSeenAt); err != nil {
			return nil, fmt.Errorf("read device: %w", err)
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (s *Store) RevokeDevice(ctx context.Context, accountID int64, deviceID string, revokedAt int64) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE devices SET revoked_at = COALESCE(revoked_at, ?) WHERE account_id = ? AND id = ?`,
		revokedAt, accountID, deviceID)
	if err != nil {
		return false, fmt.Errorf("revoke device: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("revoke device: %w", err)
	}
	return affected > 0, nil
}
