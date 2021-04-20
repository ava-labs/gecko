package health

// Check is a health check. Returns the health check results and,
// if unhealthy, a non-nil error.
type Check func() (interface{}, error)

// check wraps a Check and a name
type check struct {
	name    string
	checkFn Check
}

// Name is the identifier for this check and must be unique among all Checks
func (c *check) Name() string { return c.name }

// Execute performs the health check. It returns nil if the check passes.
// It can also return additional information to marshal and display to the caller
func (c *check) Execute() (interface{}, error) { return c.checkFn() }

// monotonicCheck is a check that will run until it passes once, and after that it will
// always pass without performing any logic. Used for bootstrapping, for example.
type monotonicCheck struct {
	passed bool
	check
}

func (mc *monotonicCheck) Execute() (interface{}, error) {
	if mc.passed {
		return nil, nil
	}
	details, pass := mc.check.Execute()
	if pass == nil {
		mc.passed = true
	}
	return details, pass
}
