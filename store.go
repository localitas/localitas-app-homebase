package homebase

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/localitas/localitas-go"
)

const DatabaseName = "homebase"

const deviceColumns = "id, node_id, name, device_type, room, vendor, model, clusters, online, virtual, source, source_id, created_at, updated_at"

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func OpenStore(coreURL, dbID, token string) (*Store, error) {
	dsn := fmt.Sprintf("%s?database_id=%s&token=%s", coreURL, dbID, token)
	db, err := sql.Open("localitas", dsn)
	if err != nil {
		return nil, fmt.Errorf("open localitas db: %w", err)
	}
	return NewStore(db), nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func scanDevice(scanner interface{ Scan(...interface{}) error }) (*Device, error) {
	var d Device
	var createdAt, updatedAt int64
	var clustersJSON string
	var online, virtual int
	err := scanner.Scan(&d.ID, &d.NodeID, &d.Name, &d.DeviceType, &d.Room, &d.Vendor, &d.Model, &clustersJSON, &online, &virtual, &d.Source, &d.SourceID, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	d.Online = online == 1
	d.Virtual = virtual == 1
	d.CreatedAt = time.Unix(createdAt, 0)
	d.UpdatedAt = time.Unix(updatedAt, 0)
	json.Unmarshal([]byte(clustersJSON), &d.Clusters)
	return &d, nil
}

func (s *Store) CreateDevice(ctx context.Context, nodeID uint64, name, deviceType, room, vendor, model string, clusters []string, virtual bool) (*Device, error) {
	id := newDeviceID()
	now := time.Now().UTC().Unix()
	clustersJSON, err := json.Marshal(clusters)
	if err != nil {
		return nil, fmt.Errorf("marshal clusters: %w", err)
	}

	virtualInt := 0
	if virtual {
		virtualInt = 1
	}

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO devices (id, node_id, name, device_type, room, vendor, model, clusters, online, virtual, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)",
		id, nodeID, name, deviceType, room, vendor, model, string(clustersJSON), virtualInt, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert device: %w", err)
	}

	return &Device{
		ID:         id,
		NodeID:     nodeID,
		Name:       name,
		DeviceType: deviceType,
		Room:       room,
		Vendor:     vendor,
		Model:      model,
		Clusters:   clusters,
		Online:     true,
		Virtual:    virtual,
		CreatedAt:  time.Unix(now, 0),
		UpdatedAt:  time.Unix(now, 0),
	}, nil
}

func (s *Store) GetDevice(ctx context.Context, id string) (*Device, error) {
	d, err := scanDevice(s.db.QueryRowContext(ctx,
		"SELECT "+deviceColumns+" FROM devices WHERE id = ?", id))
	if err != nil {
		return nil, fmt.Errorf("device %s not found", id)
	}
	return d, nil
}

func (s *Store) GetDeviceByNodeID(ctx context.Context, nodeID uint64) (*Device, error) {
	d, err := scanDevice(s.db.QueryRowContext(ctx,
		"SELECT "+deviceColumns+" FROM devices WHERE node_id = ?", nodeID))
	if err != nil {
		return nil, fmt.Errorf("device with node_id %d not found", nodeID)
	}
	return d, nil
}

func (s *Store) ListDevices(ctx context.Context) ([]*Device, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+deviceColumns+" FROM devices ORDER BY room, name")
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	return scanDeviceRows(rows)
}

func (s *Store) ListDevicesByRoom(ctx context.Context, room string) ([]*Device, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+deviceColumns+" FROM devices WHERE room = ? ORDER BY name", room)
	if err != nil {
		return nil, fmt.Errorf("list devices by room: %w", err)
	}
	defer rows.Close()
	return scanDeviceRows(rows)
}

func scanDeviceRows(rows *sql.Rows) ([]*Device, error) {
	out := make([]*Device, 0)
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) UpdateDevice(ctx context.Context, id string, update DeviceUpdate) error {
	now := time.Now().UTC().Unix()
	if update.Name != "" {
		if _, err := s.db.ExecContext(ctx, "UPDATE devices SET name = ?, updated_at = ? WHERE id = ?", update.Name, now, id); err != nil {
			return err
		}
	}
	if update.Room != "" {
		if _, err := s.db.ExecContext(ctx, "UPDATE devices SET room = ?, updated_at = ? WHERE id = ?", update.Room, now, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteDevice(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM devices WHERE id = ?", id)
	return err
}

func (s *Store) SetDeviceOnline(ctx context.Context, id string, online bool) error {
	val := 0
	if online {
		val = 1
	}
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE devices SET online = ?, updated_at = ? WHERE id = ?", val, now, id)
	return err
}

func (s *Store) NextVirtualNodeID(ctx context.Context) (uint64, error) {
	var maxID sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT MAX(node_id) FROM devices WHERE virtual = 1").Scan(&maxID)
	if err != nil {
		return 1000000, nil
	}
	if !maxID.Valid {
		return 1000000, nil
	}
	return uint64(maxID.Int64) + 1, nil
}

func (s *Store) ListRooms(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT room FROM devices WHERE room != '' ORDER BY room")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []string
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
}

func (s *Store) UpsertPluginDevice(ctx context.Context, source, sourceID, name, deviceType, vendor, model string, online bool) (*Device, error) {
	now := time.Now().UTC().Unix()
	onlineInt := 0
	if online {
		onlineInt = 1
	}

	var existingID string
	err := s.db.QueryRowContext(ctx,
		"SELECT id FROM devices WHERE source = ? AND source_id = ?", source, sourceID,
	).Scan(&existingID)

	if err == nil {
		_, err = s.db.ExecContext(ctx,
			"UPDATE devices SET name = ?, device_type = ?, vendor = ?, model = ?, online = ?, updated_at = ? WHERE id = ?",
			name, deviceType, vendor, model, onlineInt, now, existingID)
		if err != nil {
			return nil, fmt.Errorf("update plugin device: %w", err)
		}
		return s.GetDevice(ctx, existingID)
	}

	id := newDeviceID()
	nodeID, err := s.nextPluginNodeID(ctx)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO devices (id, node_id, name, device_type, room, vendor, model, clusters, online, virtual, source, source_id, created_at, updated_at) VALUES (?, ?, ?, ?, '', ?, ?, '[]', ?, 0, ?, ?, ?, ?)",
		id, nodeID, name, deviceType, vendor, model, onlineInt, source, sourceID, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert plugin device: %w", err)
	}

	return s.GetDevice(ctx, id)
}

func (s *Store) DeleteBySource(ctx context.Context, source string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM devices WHERE source = ?", source)
	return err
}

func (s *Store) MarkOfflineBySource(ctx context.Context, source string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE devices SET online = 0, updated_at = ? WHERE source = ?", now, source)
	return err
}

func (s *Store) ListDevicesBySource(ctx context.Context, source string) ([]*Device, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+deviceColumns+" FROM devices WHERE source = ? ORDER BY name", source)
	if err != nil {
		return nil, fmt.Errorf("list devices by source: %w", err)
	}
	defer rows.Close()
	return scanDeviceRows(rows)
}

func (s *Store) nextPluginNodeID(ctx context.Context) (uint64, error) {
	var maxID sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT MAX(node_id) FROM devices WHERE source != ''").Scan(&maxID)
	if err != nil || !maxID.Valid {
		return 2000000, nil
	}
	return uint64(maxID.Int64) + 1, nil
}

type PluginCredential struct {
	PluginName    string `json:"plugin_name"`
	VaultPublicID string `json:"vault_public_id"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

func (s *Store) GetPluginCredential(ctx context.Context, pluginName string) (*PluginCredential, error) {
	var pc PluginCredential
	err := s.db.QueryRowContext(ctx,
		"SELECT plugin_name, vault_public_id, created_at, updated_at FROM plugin_credentials WHERE plugin_name = ?",
		pluginName,
	).Scan(&pc.PluginName, &pc.VaultPublicID, &pc.CreatedAt, &pc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &pc, nil
}

func (s *Store) SetPluginCredential(ctx context.Context, pluginName, vaultPublicID string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO plugin_credentials (plugin_name, vault_public_id, created_at, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(plugin_name) DO UPDATE SET vault_public_id = ?, updated_at = ?",
		pluginName, vaultPublicID, now, now, vaultPublicID, now)
	return err
}

func (s *Store) DeletePluginCredential(ctx context.Context, pluginName string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM plugin_credentials WHERE plugin_name = ?", pluginName)
	return err
}

func (s *Store) ListPluginCredentials(ctx context.Context) ([]PluginCredential, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT plugin_name, vault_public_id, created_at, updated_at FROM plugin_credentials ORDER BY plugin_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PluginCredential
	for rows.Next() {
		var pc PluginCredential
		if err := rows.Scan(&pc.PluginName, &pc.VaultPublicID, &pc.CreatedAt, &pc.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return out, nil
}

func newDeviceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
