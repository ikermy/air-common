package comdb

import (
	"context"
	"database/sql"
	"testing"
)

type fakeShardResolver struct {
	called bool
	userID uint64
	db     *sql.DB
}

func (f *fakeShardResolver) DBForUser(_ context.Context, userID uint64) (*sql.DB, error) {
	f.called = true
	f.userID = userID
	return f.db, nil
}

func TestConnForUserFallback(t *testing.T) {
	primary := &sql.DB{}
	d := &DB{conn: primary}

	got, err := d.ConnForUser(context.Background(), 42)
	if err != nil {
		t.Fatalf("ConnForUser: %v", err)
	}
	if got != primary {
		t.Fatal("expected primary connection without resolver")
	}
}

func TestConnForUserUsesResolver(t *testing.T) {
	primary := &sql.DB{}
	shard := &sql.DB{}
	resolver := &fakeShardResolver{db: shard}
	d := &DB{conn: primary}
	d.SetShardResolver(resolver)

	got, err := d.ConnForUser(context.Background(), 42)
	if err != nil {
		t.Fatalf("ConnForUser: %v", err)
	}
	if !resolver.called || resolver.userID != 42 {
		t.Fatalf("resolver was not called: %+v", resolver)
	}
	if got != shard {
		t.Fatal("expected shard connection")
	}
}

func TestConnForUserZeroFallsBack(t *testing.T) {
	primary := &sql.DB{}
	resolver := &fakeShardResolver{db: &sql.DB{}}
	d := &DB{conn: primary}
	d.SetShardResolver(resolver)

	got, err := d.ConnForUser(context.Background(), 0)
	if err != nil {
		t.Fatalf("ConnForUser: %v", err)
	}
	if resolver.called {
		t.Fatal("resolver must not be called for userID=0")
	}
	if got != primary {
		t.Fatal("expected primary connection")
	}
}

func TestRefConnFallbackAndOverride(t *testing.T) {
	primary := &sql.DB{}
	d := &DB{conn: primary}
	if d.RefConn() != primary {
		t.Fatal("expected primary connection as reference fallback")
	}

	reference := &sql.DB{}
	d.SetReferenceConn(reference)
	if d.RefConn() != reference {
		t.Fatal("expected explicit reference connection")
	}
}
