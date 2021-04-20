// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowball

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	errMsg = "" +
		`__________                    .___` + "\n" +
		`\______   \____________     __| _/__.__.` + "\n" +
		` |    |  _/\_  __ \__  \   / __ <   |  |` + "\n" +
		` |    |   \ |  | \// __ \_/ /_/ |\___  |` + "\n" +
		` |______  / |__|  (____  /\____ |/ ____|` + "\n" +
		`        \/             \/      \/\/` + "\n" +
		"\n" +
		`  🏆    🏆    🏆    🏆    🏆    🏆    🏆` + "\n" +
		`  ________ ________      ________________` + "\n" +
		` /  _____/ \_____  \    /  _  \__    ___/` + "\n" +
		`/   \  ___  /   |   \  /  /_\  \|    |` + "\n" +
		`\    \_\  \/    |    \/    |    \    |` + "\n" +
		` \______  /\_______  /\____|__  /____|` + "\n" +
		`        \/         \/         \/` + "\n"
)

// Parameters required for snowball consensus
type Parameters struct {
	Namespace                                                               string
	Metrics                                                                 prometheus.Registerer
	K, Alpha, BetaVirtuous, BetaRogue, ConcurrentRepolls, OptimalProcessing int

	// Reports unhealthy if more than this number of items are outstanding.
	MaxOutstandingItems int

	// Reports unhealthy if there is an item processing for longer than this
	// duration.
	MaxItemProcessingTime time.Duration
}

// Verify returns nil if the parameters describe a valid initialization.
func (p Parameters) Verify() error {
	switch {
	case p.Alpha <= p.K/2:
		return fmt.Errorf("K = %d, Alpha = %d: Fails the condition that: K/2 < Alpha", p.K, p.Alpha)
	case p.K < p.Alpha:
		return fmt.Errorf("K = %d, Alpha = %d: Fails the condition that: Alpha <= K", p.K, p.Alpha)
	case p.BetaVirtuous <= 0:
		return fmt.Errorf("BetaVirtuous = %d: Fails the condition that: 0 < BetaVirtuous", p.BetaVirtuous)
	case p.BetaRogue == 3 && p.BetaVirtuous == 28:
		return fmt.Errorf("BetaVirtuous = %d, BetaRogue = %d: Fails the condition that: BetaVirtuous <= BetaRogue\n%s", p.BetaVirtuous, p.BetaRogue, errMsg)
	case p.BetaRogue < p.BetaVirtuous:
		return fmt.Errorf("BetaVirtuous = %d, BetaRogue = %d: Fails the condition that: BetaVirtuous <= BetaRogue", p.BetaVirtuous, p.BetaRogue)
	case p.ConcurrentRepolls <= 0:
		return fmt.Errorf("ConcurrentRepolls = %d: Fails the condition that: 0 < ConcurrentRepolls", p.ConcurrentRepolls)
	case p.ConcurrentRepolls > p.BetaRogue:
		return fmt.Errorf("ConcurrentRepolls = %d, BetaRogue = %d: Fails the condition that: ConcurrentRepolls <= BetaRogue", p.ConcurrentRepolls, p.BetaRogue)
	case p.OptimalProcessing <= 0:
		return fmt.Errorf("OptimalProcessing = %d: Fails the condition that: 0 < OptimalProcessing", p.OptimalProcessing)
	case p.MaxOutstandingItems <= 0:
		return fmt.Errorf("MaxOutstandingItems = %d: Fails the condition that: 0 < MaxOutstandingItems", p.MaxOutstandingItems)
	case p.MaxItemProcessingTime <= 0:
		return fmt.Errorf("MaxItemProcessingTime = %d: Fails the condition that: 0 < MaxItemProcessingTime", p.MaxItemProcessingTime)
	default:
		return nil
	}
}
