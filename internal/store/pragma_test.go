package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFileDB_WALForeignKeysAndBusyTimeout(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var mode, fk, busy string
	if err := db.sql.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
	if fk != "1" {
		t.Errorf("foreign_keys = %q, want 1", fk)
	}
	if busy != "5000" {
		t.Errorf("busy_timeout = %q, want 5000", busy)
	}

	err = db.OpenTunnel(&TunnelRecord{
		ID: "orphan", TokenID: "missing-token", Subdomain: "x",
		Protocol: "http", PublicURL: "http://x.test", OpenedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("inserting a tunnel for a missing token succeeded; foreign keys are not enforced")
	}
}
