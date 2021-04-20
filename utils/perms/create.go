// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package perms

import (
	"errors"
	"os"
)

// Create a file at [filename] that has [perm] permissions.
func Create(filename string, perm os.FileMode) (*os.File, error) {
	if info, err := os.Stat(filename); err == nil {
		if info.Mode() != perm {
			// The file currently has the wrong permissions, so update them.
			if err := os.Chmod(filename, perm); err != nil {
				return nil, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
}
