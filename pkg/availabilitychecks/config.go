package availabilitychecks

import (
	"time"
)

// ExtDependencyAvailabilityCheck holds fields for external dependencies
type ExtDependencyAvailabilityCheck struct {
	HTTP HTTPTargets `yaml:"http"`
}

// HTTPTargets holds fields describing http targets
type HTTPTargets struct {
	Timeout int      `yaml:"timeout" default:"15"`
	URLS    []string `yaml:"urls"`
}

// GetTimeoutDuration returns the timeout duration from the ExtDependencyAvailabilityCheck type
func (e *ExtDependencyAvailabilityCheck) GetTimeoutDuration() time.Duration {
	return time.Duration(e.HTTP.Timeout) * time.Second
}
