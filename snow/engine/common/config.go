// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/validators"
)

// Config wraps the common configurations that are needed by a Snow consensus
// engine
type Config struct {
	Ctx        *snow.Context
	Validators validators.Set
	Beacons    validators.Set

	SampleK       int
	StartupAlpha  uint64
	Alpha         uint64
	Sender        Sender
	Bootstrapable Bootstrapable
	Subnet        Subnet
	Delay         Delay

	// Should Bootstrap be retried
	RetryBootstrap bool
	// Max number of times to retry bootstrap
	RetryBootstrapMaxAttempts int
}

// Context implements the Engine interface
func (c *Config) Context() *snow.Context { return c.Ctx }

// IsBootstrapped returns true iff this chain is done bootstrapping
func (c *Config) IsBootstrapped() bool { return c.Ctx.IsBootstrapped() }
