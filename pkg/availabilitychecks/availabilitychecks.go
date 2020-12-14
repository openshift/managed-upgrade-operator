package availabilitychecks

type Config interface{}

//go:generate mockgen -destination=mocks/mockAvailabilityChecks.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks AvailabilityChecker
type AvailabilityChecker interface {
	AvailabilityCheck() error
}

type AvailabilityCheckers []AvailabilityChecker

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
