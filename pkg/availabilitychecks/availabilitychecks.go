package availabilitychecks

// Config is an interface type used for AvailabilityChecker config
type Config interface{}

// AvailabilityChecker is an interface that enables implementations of AvailabilityChecker
//go:generate mockgen -destination=mocks/mockAvailabilityChecks.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks AvailabilityChecker
type AvailabilityChecker interface {
	AvailabilityCheck() error
}

// AvailabilityCheckers is a slice of AvailabilityChecker
type AvailabilityCheckers []AvailabilityChecker

// GetAvailabilityCheckers returns a GetAvailabilityCheckers containing configured GetAvailabilityChecker
func GetAvailabilityCheckers(availCfg *ExtDependencyAvailabilityCheck) (AvailabilityCheckers, error) {
	var aCs AvailabilityCheckers

	if len(availCfg.HTTP.URLS) > 0 {
		httpConfig := HTTPConfig{
			Targets: availCfg.HTTP.URLS,
			Timeout: availCfg.GetTimeoutDuration(),
		}
		HTTPChecker, err := GetHTTPAvailabilityChecker(httpConfig)
		if err != nil {
			return aCs, err
		}
		aCs = append(aCs, HTTPChecker)
	}

	return aCs, nil
}
