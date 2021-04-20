// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package logging

import "strings"

func Sanitize(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

func SanitizeArgs(a []interface{}) []interface{} {
	for i, v := range a {
		if v, ok := v.(string); ok {
			a[i] = Sanitize(v)
		}
	}
	return a
}
