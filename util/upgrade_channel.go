package util

import (
	"fmt"
	"github.com/blang/semver"
)

// Infers a CVO channel name from the channel group and FROM/TO desired version edges
func InferUpgradeChannelFromChannelGroup(channelGroup string, fromVersion string, toVersion string) (*string, error) {

	fromSV, err := semver.Parse(fromVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid semantic FROM version: %v", fromVersion)
	}
	toSV, err := semver.Parse(toVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid semantic TO version: %v", fromVersion)
	}

	if fromSV.Major != toSV.Major {
		return nil, fmt.Errorf("FROM (%v) and TO (%v) major version must match", fromVersion, toVersion)
	}

	if fromSV.Minor == toSV.Minor {
		channel := fmt.Sprintf("%v-%v.%v", channelGroup, fromSV.Major, fromSV.Minor)
		return &channel, nil
	}

	if fromSV.Minor + 1 == toSV.Minor {
		channel := fmt.Sprintf("%v-%v.%v", channelGroup, toSV.Major, toSV.Minor)
		return &channel, nil
	}

	return nil, fmt.Errorf("FROM (%v) and TO (%v) does not appear to be a valid edge", fromVersion, toVersion)
}

