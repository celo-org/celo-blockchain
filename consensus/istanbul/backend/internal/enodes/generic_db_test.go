package enodes

import (
	"testing"

	"github.com/ethereum/go-ethereum/log"
	"github.com/syndtr/goleveldb/leveldb"
)

type mockVersionedEntry struct {
	Version uint
}

func (mve *mockVersionedEntry) GetVersion() uint {
	return mve.Version
}

func TestVersionedEntryUpsert(t *testing.T) {
	vedb, err := newVersionedEntryDB(int64(0), "", log.New(), nil)
	if err != nil {
		t.Fatal("Failed to create versioned entry DB")
	}

	type testCase struct {
		ExistingEntry                 *mockVersionedEntry
		NewEntry                      *mockVersionedEntry
		ExpectedOnExistingEntryCalled bool
		ExpectedOnNewEntryCalled      bool
	}

	testCases := []*testCase{
		&testCase{
			ExistingEntry:                 nil,
			NewEntry:                      &mockVersionedEntry{Version: 1},
			ExpectedOnExistingEntryCalled: false,
			ExpectedOnNewEntryCalled:      true,
		},
		&testCase{
			ExistingEntry:                 &mockVersionedEntry{Version: 1},
			NewEntry:                      &mockVersionedEntry{Version: 2},
			ExpectedOnExistingEntryCalled: true,
			ExpectedOnNewEntryCalled:      false,
		},
		&testCase{
			ExistingEntry:                 &mockVersionedEntry{Version: 1},
			NewEntry:                      &mockVersionedEntry{Version: 1},
			ExpectedOnExistingEntryCalled: false,
			ExpectedOnNewEntryCalled:      false,
		},
		&testCase{
			ExistingEntry:                 &mockVersionedEntry{Version: 1},
			NewEntry:                      &mockVersionedEntry{Version: 0},
			ExpectedOnExistingEntryCalled: false,
			ExpectedOnNewEntryCalled:      false,
		},
	}

	for _, testCase := range testCases {
		onExistingEntryCalled, onNewEntryCalled, err := upsertEntry(vedb, testCase.ExistingEntry, testCase.NewEntry)
		if err != nil {
			t.Fatal("Failed to upsert entry")
		}
		if testCase.ExpectedOnExistingEntryCalled != onExistingEntryCalled {
			t.Errorf("Unexpected onExistingEntryCalled value. Expected %v, got %v", testCase.ExpectedOnExistingEntryCalled, onExistingEntryCalled)
		}
		if testCase.ExpectedOnNewEntryCalled != onNewEntryCalled {
			t.Errorf("Unexpected onExistingEntryCalled value. Expected %v, got %v", testCase.ExpectedOnNewEntryCalled, onNewEntryCalled)
		}
	}
	// t.Errorf("hey")
}

func upsertEntry(vedb *versionedEntryDB, existingEntry *mockVersionedEntry, newEntry *mockVersionedEntry) (bool, bool, error) {
	var (
		onExistingEntryCalled bool
		onNewEntryCalled      bool
	)

	getExistingEntry := func(_ versionedEntry) (versionedEntry, error) {
		if existingEntry == nil {
			return nil, leveldb.ErrNotFound
		}
		return existingEntry, nil
	}
	onExistingEntry := func(_ *leveldb.Batch, _ versionedEntry, _ versionedEntry) error {
		onExistingEntryCalled = true
		return nil
	}
	onNewEntry := func(_ *leveldb.Batch, _ versionedEntry) error {
		onNewEntryCalled = true
		return nil
	}

	err := vedb.Upsert(
		[]versionedEntry{versionedEntry(newEntry)},
		getExistingEntry,
		onExistingEntry,
		onNewEntry,
	)
	return onExistingEntryCalled, onNewEntryCalled, err
}
