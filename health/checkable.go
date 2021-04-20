package health

// Checkable can have its health checked
type Checkable interface {
	// HealthCheck returns health check results and,
	// if not healthy, a non-nil error
	HealthCheck() (interface{}, error)
}
