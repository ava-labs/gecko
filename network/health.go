package network

import "time"

// HealthConfig describes parameters for network layer health checks.
type HealthConfig struct {
	// Must be connected to at least this many peers to be considered healthy
	MinConnectedPeers uint

	// Must have received a message from the network within this duration
	// to be considered healthy. Must be positive
	MaxTimeSinceMsgReceived time.Duration

	// Must have sent a message over the network within this duration
	// to be considered healthy. Must be positive
	MaxTimeSinceMsgSent time.Duration

	// If greater than this portion of the pending send byte queue is full,
	// will report unhealthy. Must be in (0,1]
	MaxPortionSendQueueBytesFull float64

	// If greater than this portion of the attempts to send a message to a peer
	// fail, will return unhealthy. Does not include send attempts that were not
	// made due to benching. Must be in [0,1]
	MaxSendFailRate float64

	// Halflife of averager used to calculate the send fail rate
	// Must be > 0.
	// Larger value --> Drop rate affected less by recent messages
	MaxSendFailRateHalflife time.Duration
}
