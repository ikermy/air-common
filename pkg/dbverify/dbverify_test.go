package dbverify

import (
	"context"
	"testing"
)

func TestCompareUserRequiresDBs(t *testing.T) {
	if _, err := CompareUser(context.Background(), nil, nil, 1, nil); err == nil {
		t.Fatal("expected error for nil databases")
	}
}

func TestDefaultTables(t *testing.T) {
	tables := DefaultTables()
	if len(tables) == 0 {
		t.Fatal("DefaultTables is empty")
	}
	seen := make(map[string]bool, len(tables))
	for _, table := range tables {
		if table.Name == "" || table.UserColumn == "" || table.KeyColumn == "" {
			t.Fatalf("incomplete table spec: %+v", table)
		}
		if seen[table.Name] {
			t.Fatalf("duplicate table: %s", table.Name)
		}
		seen[table.Name] = true
	}
}
