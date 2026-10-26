package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	_ "modernc.org/sqlite"
)

func openMemory(t *testing.T) *Store {
	t.Helper()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(%q) error: %v", ":memory:", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func countMigrations(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations error: %v", err)
	}
	return count
}

func seedAccount(t *testing.T, store *Store, username string) Account {
	t.Helper()
	account, err := store.EnsureAccount(context.Background(), username)
	if err != nil {
		t.Fatalf("EnsureAccount(%q) error: %v", username, err)
	}
	return account
}

func seedDevice(t *testing.T, store *Store, accountID int64, id string) {
	t.Helper()
	err := store.CreateDevice(context.Background(), accountID, NewDevice{ID: id, Name: id, TokenHash: "hash-" + id, CreatedAt: 1})
	if err != nil {
		t.Fatalf("CreateDevice(%d, %q) error: %v", accountID, id, err)
	}
}

func tagChange(id, deviceID string) NewChange {
	return NewChange{ID: id, Kind: "tag", RecordID: "record", Data: []byte(`{"name":"x"}`), UpdatedAt: 1, DeviceID: deviceID}
}

func TestAppliesEachMigrationOnceAcrossReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", path, err)
	}
	recorded := countMigrations(t, first)
	first.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) again error: %v", path, err)
	}
	defer reopened.Close()
	if got := countMigrations(t, reopened); recorded == 0 || got != recorded {
		t.Fatalf("migrations after reopening %q = %d, want %d (nonzero)", path, got, recorded)
	}
}

func TestEnsureAccountReturnsTheSameAccountTwice(t *testing.T) {
	store := openMemory(t)
	first := seedAccount(t, store, "reader")
	if second := seedAccount(t, store, "reader"); second != first {
		t.Fatalf("EnsureAccount(%q) again = %v, want %v", "reader", second, first)
	}
}

func TestFindsADeviceByTokenHash(t *testing.T) {
	store := openMemory(t)
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	ctx := context.Background()
	device, found, err := store.DeviceByTokenHash(ctx, "hash-phone")
	if err != nil || !found || device.ID != "phone" || device.AccountID != account.ID || device.LastSeenAt != nil {
		t.Fatalf("DeviceByTokenHash(%q) = %+v, %v, %v, want device phone of account %d, true, nil", "hash-phone", device, found, err, account.ID)
	}
	if device, found, err := store.DeviceByTokenHash(ctx, "hash-other"); err != nil || found {
		t.Fatalf("DeviceByTokenHash(%q) = %+v, %v, %v, want not found", "hash-other", device, found, err)
	}
}

func TestHidesARevokedDeviceFromTokenLookupAndList(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	found, err := store.RevokeDevice(ctx, account.ID, "phone", 5)
	if err != nil || !found {
		t.Fatalf("RevokeDevice(%d, %q, 5) = %v, %v, want true, nil", account.ID, "phone", found, err)
	}
	if device, found, err := store.DeviceByTokenHash(ctx, "hash-phone"); err != nil || found {
		t.Fatalf("DeviceByTokenHash(%q) = %+v, %v, %v, want not found", "hash-phone", device, found, err)
	}
	if listed, err := store.ListDevices(ctx, account.ID); err != nil || len(listed) != 0 {
		t.Fatalf("ListDevices(%d) = %+v, %v, want none", account.ID, listed, err)
	}
}

func TestReportsNoDeviceWhenRevokingAnUnknownId(t *testing.T) {
	store := openMemory(t)
	account := seedAccount(t, store, "reader")
	found, err := store.RevokeDevice(context.Background(), account.ID, "missing", 5)
	if err != nil || found {
		t.Fatalf("RevokeDevice(%d, %q, 5) = %v, %v, want false, nil", account.ID, "missing", found, err)
	}
}

func TestRecordsWhenADeviceWasLastSeen(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	if err := store.TouchDevice(ctx, account.ID, "phone", 42); err != nil {
		t.Fatalf("TouchDevice(%d, %q, 42) error: %v", account.ID, "phone", err)
	}
	listed, err := store.ListDevices(ctx, account.ID)
	if err != nil || len(listed) != 1 || listed[0].LastSeenAt == nil || *listed[0].LastSeenAt != 42 {
		t.Fatalf("ListDevices(%d) = %+v, %v, want one device last seen at 42", account.ID, listed, err)
	}
}

func TestReturnsTheStoredSeqForARepeatedChangeId(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	first, err := store.AppendChanges(ctx, account.ID, 10, []NewChange{tagChange("a", "phone")})
	if err != nil {
		t.Fatalf("AppendChanges(a) error: %v", err)
	}
	again, err := store.AppendChanges(ctx, account.ID, 20, []NewChange{tagChange("a", "phone"), tagChange("b", "phone")})
	if err != nil {
		t.Fatalf("AppendChanges(a, b) error: %v", err)
	}
	if want := []int64{first[0], first[0] + 1}; !slices.Equal(again, want) {
		t.Fatalf("AppendChanges(a, b) seqs = %v, want %v", again, want)
	}
	pulled, _, err := store.ChangesAfter(ctx, account.ID, 0, 10)
	if err != nil || len(pulled) != 2 || pulled[0].ReceivedAt != 10 {
		t.Fatalf("ChangesAfter(%d, 0, 10) = %+v, %v, want 2 changes, the first received at 10", account.ID, pulled, err)
	}
}

func TestGivesOneSeqToAnIdRepeatedInsideABatch(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	seqs, err := store.AppendChanges(ctx, account.ID, 10, []NewChange{tagChange("a", "phone"), tagChange("a", "phone")})
	if err != nil || len(seqs) != 2 || seqs[0] != seqs[1] {
		t.Fatalf("AppendChanges(a, a) = %v, %v, want two equal seqs, nil", seqs, err)
	}
	pulled, _, err := store.ChangesAfter(ctx, account.ID, 0, 10)
	if err != nil || len(pulled) != 1 {
		t.Fatalf("ChangesAfter(%d, 0, 10) = %+v, %v, want 1 change", account.ID, pulled, err)
	}
}

func TestNumbersChangesInRequestOrderFromOne(t *testing.T) {
	store := openMemory(t)
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	seqs, err := store.AppendChanges(context.Background(), account.ID, 10, []NewChange{tagChange("a", "phone"), tagChange("b", "phone"), tagChange("c", "phone")})
	if want := []int64{1, 2, 3}; err != nil || !slices.Equal(seqs, want) {
		t.Fatalf("AppendChanges(a, b, c) = %v, %v, want %v, nil", seqs, err, want)
	}
}

func TestReadsChangesAfterASeqUpToTheLimit(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	if _, err := store.AppendChanges(ctx, account.ID, 10, []NewChange{tagChange("a", "phone"), tagChange("b", "phone"), tagChange("c", "phone")}); err != nil {
		t.Fatalf("AppendChanges(a, b, c) error: %v", err)
	}

	page, more, err := store.ChangesAfter(ctx, account.ID, 1, 1)
	if err != nil || len(page) != 1 || page[0].ID != "b" || !more {
		t.Fatalf("ChangesAfter(%d, 1, 1) = %+v, %v, %v, want [b], true, nil", account.ID, page, more, err)
	}
	rest, more, err := store.ChangesAfter(ctx, account.ID, 1, 2)
	if err != nil || len(rest) != 2 || rest[1].ID != "c" || more {
		t.Fatalf("ChangesAfter(%d, 1, 2) = %+v, %v, %v, want [b c], false, nil", account.ID, rest, more, err)
	}
}

func TestReadsATombstoneAsNoData(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	if _, err := store.AppendChanges(ctx, account.ID, 10, []NewChange{{ID: "a", Kind: "tag", RecordID: "r", UpdatedAt: 1, DeviceID: "phone"}}); err != nil {
		t.Fatalf("AppendChanges(a) error: %v", err)
	}
	pulled, _, err := store.ChangesAfter(ctx, account.ID, 0, 10)
	if err != nil || len(pulled) != 1 || pulled[0].Data != nil {
		t.Fatalf("ChangesAfter(%d, 0, 10) = %+v, %v, want 1 change with nil Data", account.ID, pulled, err)
	}
}

func TestKeepsAnotherAccountsChangesOutOfAPull(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	first := seedAccount(t, store, "first")
	second := seedAccount(t, store, "second")
	seedDevice(t, store, first.ID, "device-first")
	seedDevice(t, store, second.ID, "device-second")
	if _, err := store.AppendChanges(ctx, second.ID, 10, []NewChange{tagChange("shared-id", "device-second")}); err != nil {
		t.Fatalf("AppendChanges(%d, shared-id) error: %v", second.ID, err)
	}
	seqs, err := store.AppendChanges(ctx, first.ID, 10, []NewChange{tagChange("shared-id", "device-first")})
	if err != nil {
		t.Fatalf("AppendChanges(%d, shared-id) error: %v", first.ID, err)
	}
	pulled, _, err := store.ChangesAfter(ctx, first.ID, 0, 10)
	if err != nil || len(pulled) != 1 || pulled[0].DeviceID != "device-first" || pulled[0].Seq != seqs[0] {
		t.Fatalf("ChangesAfter(%d, 0, 10) = %+v, %v, want only the change from device-first with seq %d", first.ID, pulled, err, seqs[0])
	}
}

func TestKeepsAnotherAccountsDevicesOutOfAListAndARevoke(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	first := seedAccount(t, store, "first")
	second := seedAccount(t, store, "second")
	seedDevice(t, store, second.ID, "device-second")
	if listed, err := store.ListDevices(ctx, first.ID); err != nil || len(listed) != 0 {
		t.Fatalf("ListDevices(%d) = %+v, %v, want none", first.ID, listed, err)
	}
	if found, err := store.RevokeDevice(ctx, first.ID, "device-second", 5); err != nil || found {
		t.Fatalf("RevokeDevice(%d, %q, 5) = %v, %v, want false, nil", first.ID, "device-second", found, err)
	}
	if err := store.TouchDevice(ctx, first.ID, "device-second", 9); err != nil {
		t.Fatalf("TouchDevice(%d, %q, 9) error: %v", first.ID, "device-second", err)
	}
	listed, err := store.ListDevices(ctx, second.ID)
	if err != nil || len(listed) != 1 || listed[0].LastSeenAt != nil {
		t.Fatalf("ListDevices(%d) = %+v, %v, want one device never seen", second.ID, listed, err)
	}
}
