// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package leveldb

import (
	"testing"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/utils/logging"
)

func TestInterface(t *testing.T) {
	for _, test := range database.Tests {
		folder := t.TempDir()
		db, err := New(folder, logging.NoLog{}, 0, 0, 0)
		if err != nil {
			t.Fatalf("leveldb.New(%q, logging.NoLog{}, 0, 0, 0) errored with %s", folder, err)
		}

		// The database may have been closed by the test, so we don't care if it
		// errors here.
		defer db.Close()

		test(t, db)
	}
}
