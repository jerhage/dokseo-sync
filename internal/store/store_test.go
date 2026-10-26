package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openMemory(t *testing.T) *Store {
	t.Helper()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func countMigrations(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func seedAccount(t *testing.T, store *Store, username string) Account {
	t.Helper()
	account, err := store.EnsureAccount(context.Background(), username)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func seedDevice(t *testing.T, store *Store, accountID int64, id string) {
	t.Helper()
	err := store.CreateDevice(context.Background(), accountID, NewDevice{ID: id, Name: id, TokenHash: "hash-" + id, CreatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
}

func tagChange(id, deviceID string) NewChange {
	return NewChange{ID: id, Kind: "tag", RecordID: "record", Data: []byte(`{"name":"x"}`), UpdatedAt: 1, DeviceID: deviceID}
}

func TestAppliesEachMigrationOnceAcrossReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	recorded := countMigrations(t, first)
	first.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if recorded == 0 || countMigrations(t, reopened) != recorded {
		t.Fatalf("recorded %d, then %d", recorded, countMigrations(t, reopened))
	}
}

func TestEnsureAccountReturnsTheSameAccountTwice(t *testing.T) {
	store := openMemory(t)
	first := seedAccount(t, store, "reader")
	if second := seedAccount(t, store, "reader"); second != first {
		t.Fatalf("got %v then %v", first, second)
	}
}

func TestFindsADeviceByTokenHash(t *testing.T) {
	store := openMemory(t)
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	device, found, err := store.DeviceByTokenHash(context.Background(), "hash-phone")
	if err != nil || !found || device.ID != "phone" || device.AccountID != account.ID || device.LastSeenAt != nil {
		t.Fatalf("device %+v found %v error %v", device, found, err)
	}
	if _, found, _ := store.DeviceByTokenHash(context.Background(), "hash-other"); found {
		t.Fatal("found an unknown token")
	}
}

func TestHidesARevokedDeviceFromTokenLookupAndList(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	found, err := store.RevokeDevice(ctx, account.ID, "phone", 5)
	if err != nil || !found {
		t.Fatalf("found %v error %v", found, err)
	}
	if _, found, _ := store.DeviceByTokenHash(ctx, "hash-phone"); found {
		t.Fatal("found a revoked token")
	}
	if listed, _ := store.ListDevices(ctx, account.ID); len(listed) != 0 {
		t.Fatalf("listed %v", listed)
	}
}

func TestReportsNoDeviceWhenRevokingAnUnknownId(t *testing.T) {
	store := openMemory(t)
	account := seedAccount(t, store, "reader")
	found, err := store.RevokeDevice(context.Background(), account.ID, "missing", 5)
	if err != nil || found {
		t.Fatalf("found %v error %v", found, err)
	}
}

func TestRecordsWhenADeviceWasLastSeen(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	if err := store.TouchDevice(ctx, account.ID, "phone", 42); err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListDevices(ctx, account.ID)
	if err != nil || len(listed) != 1 || listed[0].LastSeenAt == nil || *listed[0].LastSeenAt != 42 {
		t.Fatalf("listed %+v error %v", listed, err)
	}
}

func TestReturnsTheStoredSeqForARepeatedChangeId(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	first, err := store.AppendChanges(ctx, account.ID, 10, []NewChange{tagChange("a", "phone")})
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.AppendChanges(ctx, account.ID, 20, []NewChange{tagChange("a", "phone"), tagChange("b", "phone")})
	if err != nil {
		t.Fatal(err)
	}
	if again[0] != first[0] || again[1] != first[0]+1 {
		t.Fatalf("first %v again %v", first, again)
	}
	pulled, _, _ := store.ChangesAfter(ctx, account.ID, 0, 10)
	if len(pulled) != 2 || pulled[0].ReceivedAt != 10 {
		t.Fatalf("pulled %+v", pulled)
	}
}

func TestGivesOneSeqToAnIdRepeatedInsideABatch(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	seqs, err := store.AppendChanges(ctx, account.ID, 10, []NewChange{tagChange("a", "phone"), tagChange("a", "phone")})
	if err != nil || seqs[0] != seqs[1] {
		t.Fatalf("seqs %v error %v", seqs, err)
	}
	pulled, _, _ := store.ChangesAfter(ctx, account.ID, 0, 10)
	if len(pulled) != 1 {
		t.Fatalf("stored %d changes", len(pulled))
	}
}

func TestNumbersChangesInRequestOrderFromOne(t *testing.T) {
	store := openMemory(t)
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	seqs, err := store.AppendChanges(context.Background(), account.ID, 10, []NewChange{tagChange("a", "phone"), tagChange("b", "phone"), tagChange("c", "phone")})
	if err != nil || seqs[0] != 1 || seqs[1] != 2 || seqs[2] != 3 {
		t.Fatalf("seqs %v error %v", seqs, err)
	}
}

func TestReadsChangesAfterASeqUpToTheLimit(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	store.AppendChanges(ctx, account.ID, 10, []NewChange{tagChange("a", "phone"), tagChange("b", "phone"), tagChange("c", "phone")})

	page, more, err := store.ChangesAfter(ctx, account.ID, 1, 1)
	if err != nil || len(page) != 1 || page[0].ID != "b" || !more {
		t.Fatalf("page %+v more %v error %v", page, more, err)
	}
	rest, more, _ := store.ChangesAfter(ctx, account.ID, 1, 2)
	if len(rest) != 2 || rest[1].ID != "c" || more {
		t.Fatalf("rest %+v more %v", rest, more)
	}
}

func TestReadsATombstoneAsNoData(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	account := seedAccount(t, store, "reader")
	seedDevice(t, store, account.ID, "phone")
	store.AppendChanges(ctx, account.ID, 10, []NewChange{{ID: "a", Kind: "tag", RecordID: "r", UpdatedAt: 1, DeviceID: "phone"}})
	pulled, _, _ := store.ChangesAfter(ctx, account.ID, 0, 10)
	if len(pulled) != 1 || pulled[0].Data != nil {
		t.Fatalf("pulled %+v", pulled)
	}
}

func TestKeepsAnotherAccountsChangesOutOfAPull(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	first := seedAccount(t, store, "first")
	second := seedAccount(t, store, "second")
	seedDevice(t, store, first.ID, "device-first")
	seedDevice(t, store, second.ID, "device-second")
	store.AppendChanges(ctx, second.ID, 10, []NewChange{tagChange("shared-id", "device-second")})
	seqs, err := store.AppendChanges(ctx, first.ID, 10, []NewChange{tagChange("shared-id", "device-first")})
	if err != nil {
		t.Fatal(err)
	}
	pulled, _, _ := store.ChangesAfter(ctx, first.ID, 0, 10)
	if len(pulled) != 1 || pulled[0].DeviceID != "device-first" || pulled[0].Seq != seqs[0] {
		t.Fatalf("pulled %+v", pulled)
	}
}

func TestKeepsAnotherAccountsDevicesOutOfAListAndARevoke(t *testing.T) {
	store := openMemory(t)
	ctx := context.Background()
	first := seedAccount(t, store, "first")
	second := seedAccount(t, store, "second")
	seedDevice(t, store, second.ID, "device-second")
	if listed, _ := store.ListDevices(ctx, first.ID); len(listed) != 0 {
		t.Fatalf("listed %v", listed)
	}
	if found, _ := store.RevokeDevice(ctx, first.ID, "device-second", 5); found {
		t.Fatal("revoked another account's device")
	}
	if err := store.TouchDevice(ctx, first.ID, "device-second", 9); err != nil {
		t.Fatal(err)
	}
	if listed, _ := store.ListDevices(ctx, second.ID); listed[0].LastSeenAt != nil {
		t.Fatal("touched another account's device")
	}
}
