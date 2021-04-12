package collector_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"k8s.io/apimachinery/pkg/types"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/collector"
	mockUCMgr "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

const metadata = `
		# HELP muo_upgrade_state_timestamp Timestampes of upgrade state execution
		# TYPE muo_upgrade_state_timestamp gauge
	`

var _ = Describe("Collector", func() {
	var (
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
		uCollector        prometheus.Collector

		mockUpgradeConfigManager *mockUCMgr.MockUpgradeConfigManager
		mockKubeClient           *mocks.MockClient
		mockCtrl                 *gomock.Controller
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseNew).GetUpgradeConfig()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		uCollector, _ = collector.NewUpgradeCollector(mockKubeClient)
		mockUpgradeConfigManager = mockUCMgr.NewMockUpgradeConfigManager(mockCtrl)

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("Collecting Upgrade Status to write metrics", func() {
		Context("upgrade is scheduled", func() {
			var (
				expected  string
				upgradeAt time.Time
			)
			BeforeEach(func() {
				//		upgradeAt = time.Now()
				//		upgradeConfig.Spec.Desired.Version = "4.4.4"
				//		upgradeConfig.Spec.UpgradeAt = upgradeAt.Format(time.RFC3339)
			})
			It("should write a metric for upgradeAt time", func() {
				expected = fmt.Sprintf(`

								muo_upgrade_state_timestamp{version="%s",phase="%s"} %f
								`, upgradeConfig.Spec.Desired.Version, "pending", float64(upgradeAt.Unix()))
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(upgradeConfig, nil),
				)
				//				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), "muo_upgrade_state_timestamp")
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected))
				Expect(err).To(BeNil())
			})
		})
	})
})
