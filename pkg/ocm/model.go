package ocm

import (
	"bytes"
	"encoding/json"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

// UpgradePolicy preserves architecture until the OCM SDK exposes the field.
// All existing policy fields continue to use the SDK's decoding and accessors.
type UpgradePolicy struct {
	*cmv1.UpgradePolicy
	Architecture string
}

// UnmarshalJSON decodes the policy without discarding the architecture field.
func (p *UpgradePolicy) UnmarshalJSON(data []byte) error {
	var fields struct {
		Architecture string `json:"architecture"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	policy, err := cmv1.UnmarshalUpgradePolicy(bytes.NewReader(data))
	if err != nil {
		return err
	}
	p.UpgradePolicy = policy
	p.Architecture = fields.Architecture
	return nil
}

// UpgradePolicyList is the policy response including fields not yet in the SDK.
type UpgradePolicyList struct {
	Items []*UpgradePolicy `json:"items"`
	Total int              `json:"total"`
}
