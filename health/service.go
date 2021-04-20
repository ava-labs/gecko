package health

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/prometheus/client_golang/prometheus"

	health "github.com/AppsFlyer/go-sundheit"
)

// Service performs health checks. Other things register health checks
// with Service, which performs them.
type Service interface {
	RegisterCheck(name string, checkFn Check) error
	RegisterMonotonicCheck(name string, checkFn Check) error
	Results() (map[string]health.Result, bool)
}

// NewService returns a new [Service] where the health checks
// run every [checkFreq]
func NewService(checkFreq time.Duration, log logging.Logger, namespace string, registry prometheus.Registerer) (Service, error) {
	healthChecker := health.New()
	metrics, err := newMetrics(log, namespace, registry)
	if err != nil {
		return nil, err
	}
	// Add the check listener to report when a check changes status.
	healthChecker.WithCheckListener(&checkListener{
		log:     log,
		checks:  make(map[string]bool),
		metrics: metrics,
	})
	return &service{
		Health:    healthChecker,
		checkFreq: checkFreq,
	}, nil
}

// service implements Service
type service struct {
	// performs the underlying health checks
	health.Health
	// Time between health checks
	checkFreq time.Duration
}

// RegisterCheckFn adds a check that calls [checkFn] to evaluate health
func (s *service) RegisterCheck(name string, checkFn Check) error {
	check := &check{
		name:    name,
		checkFn: checkFn,
	}

	return s.Health.RegisterCheck(&health.Config{
		InitialDelay:    constants.DefaultHealthCheckInitialDelay,
		ExecutionPeriod: s.checkFreq,
		Check:           check,
	})
}

// RegisterMonotonicCheckFn adds a health check that, after it passes once,
// always returns healthy without executing any logic
func (s *service) RegisterMonotonicCheck(name string, checkFn Check) error {
	c := &monotonicCheck{
		check: check{
			name:    name,
			checkFn: checkFn,
		},
	}

	return s.Health.RegisterCheck(&health.Config{
		InitialDelay:    constants.DefaultHealthCheckInitialDelay,
		ExecutionPeriod: s.checkFreq,
		Check:           c,
	})
}

type checkListener struct {
	log logging.Logger

	// lock ensures that updates and reads to [checks] are atomic
	lock sync.Mutex
	// checks maps name -> is healthy
	checks  map[string]bool
	metrics *metrics
}

func (c *checkListener) OnCheckStarted(name string) {
	c.log.Debug("starting to run %s", name)
}

// OnCheckCompleted is called concurrently with multiple health checks.
func (c *checkListener) OnCheckCompleted(name string, result health.Result) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		c.log.Error("failed to encode %q when it was failing due to: %s", name, err)
		return
	}

	isHealthy := result.IsHealthy()

	c.lock.Lock()
	previouslyHealthy, exists := c.checks[name]
	c.checks[name] = isHealthy
	c.lock.Unlock()

	if !exists && !isHealthy {
		c.metrics.unHealthy()
	}

	if !exists || isHealthy == previouslyHealthy {
		if isHealthy {
			c.log.Debug("%q returned healthy with: %s", name, string(resultJSON))
		} else {
			c.log.Debug("%q returned unhealthy with: %s", name, string(resultJSON))
		}
		return
	}

	if isHealthy {
		c.log.Info("%q became healthy with: %s", name, string(resultJSON))
		c.metrics.healthy()
	} else {
		c.log.Warn("%q became unhealthy with: %s", name, string(resultJSON))
		c.metrics.unHealthy()
	}
}
