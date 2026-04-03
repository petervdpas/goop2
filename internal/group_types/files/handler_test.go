package files

import (
	"testing"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/storage"
)

func TestHandlerFlags(t *testing.T) {
	h := &Handler{}
	if !h.Flags().HostCanJoin {
		t.Fatal("files handler should allow host to join")
	}
}

func TestHandlerRegistration(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	grpMgr := group.NewTestManager(db, "self")
	defer grpMgr.Close()

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	New(nil, grpMgr, store)

	if err := grpMgr.CreateGroup("f1", "Docs", "files", "shared", 0, false); err != nil {
		t.Fatal(err)
	}

	flags := grpMgr.TypeFlagsForGroup("f1")
	if !flags.HostCanJoin {
		t.Fatal("files group should allow host join via registered handler")
	}
}
