package folders

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/cristian/holocron/internal/db"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewStore(database)
}

// TestManagedFoldersFollowTheServer: the list becomes exactly what the server
// says, a folder that keeps its path keeps its id (and so its cached scan), a
// missing path does not stop it — a USB disk can be off the bus at boot — and
// the form can no longer add or remove.
func TestManagedFoldersFollowTheServer(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()
	keep := t.TempDir()
	gone := t.TempDir()
	keepID, err := s.Add(ctx, "Viejo nombre", keep, PurposeMovies)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(ctx, "Sobra", gone, PurposeDisk); err != nil {
		t.Fatal(err)
	}

	err = s.Manage(ctx, []Spec{
		{Label: "Películas", Purpose: PurposeMovies, Path: keep},
		{Label: "Disco4", Purpose: PurposeDisk, Path: "/mnt/no-montado-todavia"},
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("folders = %+v", list)
	}
	for _, f := range list {
		if f.Path == keep && (f.ID != keepID || f.Label != "Películas") {
			t.Errorf("the kept folder changed id or label: %+v (want id %d)", f, keepID)
		}
		if f.Path == gone {
			t.Error("a folder the server no longer lists is still there")
		}
	}
	if _, err := s.Add(ctx, "x", t.TempDir(), PurposeDisk); !errors.Is(err, ErrManaged) {
		t.Errorf("Add on managed folders: %v", err)
	}
	if err := s.Delete(ctx, keepID); !errors.Is(err, ErrManaged) {
		t.Errorf("Delete on managed folders: %v", err)
	}
}
