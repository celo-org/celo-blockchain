package db

import (
	"testing"

	"github.com/celo-org/celo-blockchain/log"
	"github.com/syndtr/goleveldb/leveldb"
)

type mockEntry struct{}

func TestUpsert(t *testing.T) {
	gdb, err := New(int64(0), "", log.New(), nil)
	if err != nil {
		t.Fatal("Failed to create DB")
	}

	type testCase struct {
		ExistingEntry                 *mockEntry
		NewEntry                      *mockEntry
		ExpectedOnExistingEntryCalled bool
		ExpectedOnNewEntryCalled      bool
	}

	testCases := []*testCase{
		{
			ExistingEntry:                 nil,
			NewEntry:                      &mockEntry{},
			ExpectedOnExistingEntryCalled: false,
			ExpectedOnNewEntryCalled:      true,
		},
		{
			ExistingEntry:                 &mockEntry{},
			NewEntry:                      &mockEntry{},
			ExpectedOnExistingEntryCalled: true,
			ExpectedOnNewEntryCalled:      false,
		},
	}

	for i, testCase := range testCases {
		onExistingEntryCalled, onNewEntryCalled, err := upsertEntry(gdb, testCase.ExistingEntry, testCase.NewEntry)
		if err != nil {
			t.Fatal("Failed to upsert entry")
		}
		if testCase.ExpectedOnExistingEntryCalled != onExistingEntryCalled {
			t.Errorf("Unexpected onExistingEntryCalled value for test case %d. Expected %v, got %v", i, testCase.ExpectedOnExistingEntryCalled, onExistingEntryCalled)
		}
		if testCase.ExpectedOnNewEntryCalled != onNewEntryCalled {
			t.Errorf("Unexpected onExistingEntryCalled value for test case %d. Expected %v, got %v", i, testCase.ExpectedOnNewEntryCalled, onNewEntryCalled)
		}
	}
}

func upsertEntry(gdb *GenericDB, existingEntry *mockEntry, newEntry *mockEntry) (bool, bool, error) {
	var (
		onExistingEntryCalled bool
		onNewEntryCalled      bool
	)

	getExistingEntry := func(_ GenericEntry) (GenericEntry, error) {
		if existingEntry == nil {
			return nil, leveldb.ErrNotFound
		}
		return existingEntry, nil
	}
	onExistingEntry := func(_ *leveldb.Batch, _ GenericEntry, _ GenericEntry) error {
		onExistingEntryCalled = true
		return nil
	}
	onNewEntry := func(_ *leveldb.Batch, _ GenericEntry) error {
		onNewEntryCalled = true
		return nil
	}

	err := gdb.Upsert(
		[]GenericEntry{GenericEntry(newEntry)},
		getExistingEntry,
		onExistingEntry,
		onNewEntry,
	)
	return onExistingEntryCalled, onNewEntryCalled, err
}
