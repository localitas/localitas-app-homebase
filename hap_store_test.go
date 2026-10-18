package homebase

import (
	"database/sql"
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func migratedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", t.TempDir()+"/homebase.sqlitedb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	entries, err := fs.ReadDir(migrationsFS, "_migrations")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := fs.ReadFile(migrationsFS, "_migrations/"+f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("migration %s: %v", f, err)
		}
	}
	return db
}

// HomeKit pairing state lives in the app database: a bridge started again
// over the same database finds the pairings an earlier one stored.
func TestHAPStore_PairingsOutliveTheBridge(t *testing.T) {
	db := migratedDB(t)
	first := &hapStore{db: db}
	for key, val := range map[string]string{
		"uuid":                "bridge-uuid",
		"ios-1.pairing":       "alice's iPhone",
		"ios-2.pairing":       "bob's iPad",
		"ios_x%y.pairing":     "odd name",
		"keypair":             "secret",
		"not-a-pairing.extra": "x",
		"pairing":             "too short to match .pairing",
	} {
		if err := first.Set(key, []byte(val)); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.Set("ios-2.pairing", []byte("bob's new iPad")); err != nil {
		t.Fatal(err)
	}

	again := &hapStore{db: db}
	if got, err := again.Get("ios-2.pairing"); err != nil || string(got) != "bob's new iPad" {
		t.Fatalf("pairing after a restart: %q %v", got, err)
	}
	keys, err := again.KeysWithSuffix(".pairing")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ios-1.pairing", "ios-2.pairing", "ios_x%y.pairing"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("pairings = %v, want %v", keys, want)
	}
	if err := again.Delete("ios-1.pairing"); err != nil {
		t.Fatal(err)
	}
	if _, err := again.Get("ios-1.pairing"); err == nil {
		t.Fatal("a removed pairing is still found")
	}
}
