package benchlist

import (
	"errors"
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/validators"
)

var (
	errUnknownValidators = errors.New("unknown validator set for provided chain")
)

// Manager provides an interface for a benchlist to register whether
// queries have been successful or unsuccessful and place validators with
// consistently failing queries on a benchlist to prevent waiting up to
// the full network timeout for their responses.
type Manager interface {
	// RegisterResponse registers that we receive a request response from [validatorID]
	// regarding [chainID] within the timeout
	RegisterResponse(chainID ids.ID, validatorID ids.ShortID)
	// RegisterFailure registers that a request to [validatorID] regarding
	// [chainID] timed out
	RegisterFailure(chainID ids.ID, validatorID ids.ShortID)
	// RegisterChain registers a new chain with metrics under [namespace]
	RegisterChain(ctx *snow.Context, namespace string) error
	// IsBenched returns true if messages to [validatorID] regarding chain [chainID]
	// should not be sent over the network and should immediately fail.
	// Returns false if such messages should be sent, or if the chain is unknown.
	IsBenched(validatorID ids.ShortID, chainID ids.ID) bool
	// GetBenched returns an array of chainIDs where the specified
	// [validatorID] is benched. If called on an id.ShortID that does
	// not map to a validator, it will return an empty array.
	GetBenched(validatorID ids.ShortID) []ids.ID
}

// Config defines the configuration for a benchlist
type Config struct {
	Validators             validators.Manager
	Threshold              int
	MinimumFailingDuration time.Duration
	Duration               time.Duration
	MaxPortion             float64
	PeerSummaryEnabled     bool
}

type manager struct {
	config *Config
	// Chain ID --> benchlist for that chain.
	// Each benchlist is safe for concurrent access.
	chainBenchlists map[ids.ID]Benchlist

	lock sync.RWMutex
}

// NewManager returns a manager for chain-specific query benchlisting
func NewManager(config *Config) Manager {
	// If the maximum portion of validators allowed to be benchlisted
	// is 0, return the no-op benchlist
	if config.MaxPortion <= 0 {
		return NewNoBenchlist()
	}
	return &manager{
		config:          config,
		chainBenchlists: make(map[ids.ID]Benchlist),
	}
}

// IsBenched returns true if messages to [validatorID] regarding [chainID]
// should not be sent over the network and should immediately fail.
func (m *manager) IsBenched(validatorID ids.ShortID, chainID ids.ID) bool {
	m.lock.RLock()
	benchlist, exists := m.chainBenchlists[chainID]
	m.lock.RUnlock()

	if !exists {
		return false
	}
	isBenched := benchlist.IsBenched(validatorID)
	return isBenched
}

// GetBenched returns an array of chainIDs where the specified
// [validatorID] is benched. If called on an id.ShortID that does
// not map to a validator, it will return an empty array.
func (m *manager) GetBenched(validatorID ids.ShortID) []ids.ID {
	m.lock.RLock()
	defer m.lock.RUnlock()

	benched := []ids.ID{}
	for chainID, benchlist := range m.chainBenchlists {
		if !benchlist.IsBenched(validatorID) {
			continue
		}
		benched = append(benched, chainID)
	}
	return benched
}

func (m *manager) RegisterChain(ctx *snow.Context, namespace string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if _, exists := m.chainBenchlists[ctx.ChainID]; exists {
		return nil
	}

	vdrs, ok := m.config.Validators.GetValidators(ctx.SubnetID)
	if !ok {
		return errUnknownValidators
	}

	benchlist, err := NewBenchlist(
		ctx.Log,
		vdrs,
		m.config.Threshold,
		m.config.MinimumFailingDuration,
		m.config.Duration,
		m.config.MaxPortion,
		namespace,
		ctx.Metrics,
	)
	if err != nil {
		return err
	}

	m.chainBenchlists[ctx.ChainID] = benchlist
	return nil
}

// RegisterResponse implements the Manager interface
func (m *manager) RegisterResponse(chainID ids.ID, validatorID ids.ShortID) {
	m.lock.RLock()
	benchlist, exists := m.chainBenchlists[chainID]
	m.lock.RUnlock()

	if !exists {
		return
	}
	benchlist.RegisterResponse(validatorID)
}

// RegisterFailure implements the Manager interface
func (m *manager) RegisterFailure(chainID ids.ID, validatorID ids.ShortID) {
	m.lock.RLock()
	benchlist, exists := m.chainBenchlists[chainID]
	m.lock.RUnlock()

	if !exists {
		return
	}
	benchlist.RegisterFailure(validatorID)
}

type noBenchlist struct{}

// NewNoBenchlist returns an empty benchlist that will never stop any queries
func NewNoBenchlist() Manager { return &noBenchlist{} }

func (noBenchlist) RegisterChain(*snow.Context, string) error { return nil }
func (noBenchlist) RegisterResponse(ids.ID, ids.ShortID)      {}
func (noBenchlist) RegisterFailure(ids.ID, ids.ShortID)       {}
func (noBenchlist) IsBenched(ids.ShortID, ids.ID) bool        { return false }
func (noBenchlist) GetBenched(ids.ShortID) []ids.ID           { return nil }
