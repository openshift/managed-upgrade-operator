package availabilitychecks

type Config interface{}

//go:generate mockgen -destination=mocks/mockAvailabilityChecks.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks AvailabilityChecker
type AvailabilityChecker interface {
	AvailabilityCheck() error
}

func GetAvailabilityCheckers(availCfg *ExtDependencyAvailabilityCheck) ([]AvailabilityChecker, error) {
	AvailabilityCheckers := []AvailabilityChecker{}

	if len(availCfg.HTTP.URLS) > 0 {
		httpConfig := HTTPConfig{
			Targets: availCfg.HTTP.URLS,
			Timeout: availCfg.GetTimeoutDuration(),
		}
		HTTPChecker, err := GetHTTPAvailabilityChecker(httpConfig)
		if err != nil {
			return AvailabilityCheckers, err
		}
		AvailabilityCheckers = append(AvailabilityCheckers, HTTPChecker)
	}

	return AvailabilityCheckers, nil
}
