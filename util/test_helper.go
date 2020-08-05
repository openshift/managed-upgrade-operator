package util

import (
	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

func ExpectGetClusterVersion(m *mocks.MockClient, cv *configv1.ClusterVersionList, withErr error) {
	cvList := m.EXPECT().List(gomock.Any(), gomock.Any())
	if cv != nil {
		cvList.SetArg(1, *cv)
	}
	if withErr != nil {
		cvList.Return(withErr)
	}
}
