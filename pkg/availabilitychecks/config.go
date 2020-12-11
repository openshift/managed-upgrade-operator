package availabilitychecks

import (
	"time"
)

type ExtDependencyAvailabilityCheck struct {
	HTTP HTTPTargets `yaml:"http"`
}

type HTTPTargets struct {
	Timeout int      `yaml:"timeout" default:"15"`
	URLS    []string `yaml:"urls"`
}

func (e *ExtDependencyAvailabilityCheck) GetTimeoutDuration() time.Duration {
	return time.Duration(e.HTTP.Timeout) * time.Second
}
