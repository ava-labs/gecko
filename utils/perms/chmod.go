// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package perms

import (
	"errors"
	"os"
	"path/filepath"
)

// ChmodR sets the permissions of all directories and optionally files to [perm]
// permissions.
func ChmodR(dir string, dirOnly bool, perm os.FileMode) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.Walk(dir, func(name string, info os.FileInfo, err error) error {
		if err != nil || (dirOnly && !info.IsDir()) {
			return err
		}
		return os.Chmod(name, perm)
	})
}
