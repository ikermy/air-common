package migrate

import (
	"strings"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 5 {
		t.Fatalf("expected at least 5 migrations, got %d", len(migrations))
	}
	if migrations[0].Version != 1 || migrations[1].Version != 2 || migrations[2].Version != 3 || migrations[3].Version != 4 || migrations[4].Version != 5 {
		t.Fatalf("unexpected versions: %d, %d, %d, %d, %d", migrations[0].Version, migrations[1].Version, migrations[2].Version, migrations[3].Version, migrations[4].Version)
	}
	for _, m := range migrations {
		if strings.TrimSpace(m.Up) == "" {
			t.Fatalf("migration %d has empty up", m.Version)
		}
		if strings.TrimSpace(m.Down) == "" {
			t.Fatalf("migration %d has empty down", m.Version)
		}
		if len(m.Checksum) != 64 {
			t.Fatalf("migration %d checksum length = %d", m.Version, len(m.Checksum))
		}
	}
}

func TestSplitStatements(t *testing.T) {
	src := `
-- leading comment
ALTER TABLE t ADD COLUMN a INT;
# hash comment
INSERT INTO t (a) VALUES ('semi;colon');
/* block ; comment */
INSERT INTO t (a) VALUES ("double;quote");
ALTER TABLE t ADD COLUMN ` + "`back;tick`" + ` INT;
`
	got, err := splitStatements(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ALTER TABLE t ADD COLUMN a INT",
		"INSERT INTO t (a) VALUES ('semi;colon')",
		"INSERT INTO t (a) VALUES (\"double;quote\")",
		"ALTER TABLE t ADD COLUMN `back;tick` INT",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d statements, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("statement %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitStatementsRejectsUnterminatedLiteral(t *testing.T) {
	if _, err := splitStatements("SELECT 'oops"); err == nil {
		t.Fatal("expected unterminated literal error")
	}
}
