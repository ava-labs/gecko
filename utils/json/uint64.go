// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package json

import "strconv"

// Uint64 ...
type Uint64 uint64

// MarshalJSON ...
func (u Uint64) MarshalJSON() ([]byte, error) {
	return []byte("\"" + strconv.FormatUint(uint64(u), 10) + "\""), nil
}

// UnmarshalJSON ...
func (u *Uint64) UnmarshalJSON(b []byte) error {
	str := string(b)
	if str == Null {
		return nil
	}
	if len(str) >= 2 {
		if lastIndex := len(str) - 1; str[0] == '"' && str[lastIndex] == '"' {
			str = str[1:lastIndex]
		}
	}
	val, err := strconv.ParseUint(str, 10, 64)
	*u = Uint64(val)
	return err
}
