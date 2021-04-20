// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package version

import (
	"errors"
	"time"

	"github.com/ava-labs/avalanchego/utils/timer"
)

var (
	errIncompatible = errors.New("peers version is incompatible")
	errMaskable     = errors.New("peers version is maskable")
)

// Compatibility a utility for checking the compatibility of peer versions
type Compatibility interface {
	// Returns the local version
	Version() Version

	// Returns nil if the provided version is able to connect with the local
	// version. This means that the node will keep connections open with the
	// peer.
	Connectable(Version) error

	// Returns nil if the provided version is compatible with the local version.
	// This means that the version is connectable and that consensus messages
	// can be made to them.
	Compatible(Version) error

	// Returns nil if the provided version shouldn't be masked. This means that
	// the version is connectable but not compatible. The version is so old that
	// it should just be masked.
	Unmaskable(Version) error

	// Returns nil if the provided version will not be masked by this version.
	WontMask(Version) error

	// Returns when additional masking will occur.
	MaskTime() time.Time
}

type compatibility struct {
	version Version

	minCompatable     Version
	minCompatableTime time.Time
	prevMinCompatable Version

	minUnmaskable     Version
	minUnmaskableTime time.Time
	prevMinUnmaskable Version

	clock timer.Clock
}

// NewCompatibility returns a compatibility checker with the provided options
func NewCompatibility(
	version Version,
	minCompatable Version,
	minCompatableTime time.Time,
	prevMinCompatable Version,
	minUnmaskable Version,
	minUnmaskableTime time.Time,
	prevMinUnmaskable Version,
) Compatibility {
	return &compatibility{
		version:           version,
		minCompatable:     minCompatable,
		minCompatableTime: minCompatableTime,
		prevMinCompatable: prevMinCompatable,
		minUnmaskable:     minUnmaskable,
		minUnmaskableTime: minUnmaskableTime,
		prevMinUnmaskable: prevMinUnmaskable,
	}
}

func (c *compatibility) Version() Version { return c.version }

func (c *compatibility) Connectable(peer Version) error {
	return c.version.Compatible(peer)
}

func (c *compatibility) Compatible(peer Version) error {
	if err := c.Connectable(peer); err != nil {
		return err
	}

	if !peer.Before(c.minCompatable) {
		// The peer is at least the minimum compatible version.
		return nil
	}

	// The peer is going to be marked as incompatible at [c.minCompatableTime].
	now := c.clock.Time()
	if !now.Before(c.minCompatableTime) {
		return errIncompatible
	}

	// The minCompatable check isn't being enforced yet.
	if !peer.Before(c.prevMinCompatable) {
		// The peer is at least the previous minimum compatible version.
		return nil
	}
	return errIncompatible
}

func (c *compatibility) Unmaskable(peer Version) error {
	if err := c.Connectable(peer); err != nil {
		return err
	}

	if !peer.Before(c.minUnmaskable) {
		// The peer is at least the minimum unmaskable version.
		return nil
	}

	// The peer is going to be marked as maskable at [c.minUnmaskableTime].
	now := c.clock.Time()
	if !now.Before(c.minUnmaskableTime) {
		return errMaskable
	}

	// The minCompatable check isn't being enforced yet.
	if !peer.Before(c.prevMinUnmaskable) {
		// The peer is at least the previous minimum unmaskable version.
		return nil
	}
	return errMaskable
}

func (c *compatibility) WontMask(peer Version) error {
	if err := c.Connectable(peer); err != nil {
		return err
	}

	if !peer.Before(c.minUnmaskable) {
		// The peer is at least the minimum unmaskable version.
		return nil
	}
	return errMaskable
}

func (c *compatibility) MaskTime() time.Time {
	return c.minUnmaskableTime
}
