package collector

import (
	"os"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	ucMgrMock "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
)

const (
	TEST_OPERATOR_NAMESPACE = "test-namespace"
	TEST_UPGRADECONFIG_CR   = "test-upgrade-config"
	TEST_UPGRADE_VERSION    = "4.4.4"
	TEST_UPGRADE_CHANNEL    = "stable-4.4"
	TEST_UPGRADE_TIME       = "2020-06-20T00:00:00Z"
	TEST_UPGRADE_PDB_TIME   = 60
	TEST_UPGRADE_TYPE       = "OSD"
)

var _ = Describe("Upgrade Conditions Collector", func() {

	var (
		upgradeConfig    upgradev1alpha1.UpgradeConfig
		upgradeCollector prometheus.Collector

		// mockKubeClient           *mocks.MockClient
		mockCtrl                 *gomock.Controller
		mockUpgradeConfigManager *ucMgrMock.MockUpgradeConfigManager
		mockCVClient             *cvMocks.MockClusterVersion
		cv                       configv1.ClusterVersion

		// expected       string
		// metadata       string

		testTime time.Time
	)

	BeforeEach(func() {
		_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		mockCtrl = gomock.NewController(GinkgoT())
		// mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockUpgradeConfigManager = ucMgrMock.NewMockUpgradeConfigManager(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		testTime = time.Now()

		upgradeConfig = upgradev1alpha1.UpgradeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			},
			Spec: upgradev1alpha1.UpgradeConfigSpec{
				Desired: upgradev1alpha1.Update{
					Version: TEST_UPGRADE_VERSION,
					Channel: TEST_UPGRADE_CHANNEL,
				},
				UpgradeAt:            TEST_UPGRADE_TIME,
				PDBForceDrainTimeout: TEST_UPGRADE_PDB_TIME,
				Type:                 TEST_UPGRADE_TYPE,
			},
			Status: upgradev1alpha1.UpgradeConfigStatus{
				History: []upgradev1alpha1.UpgradeHistory{
					{
						Version: TEST_UPGRADE_VERSION,
						Phase:   upgradev1alpha1.UpgradePhaseUpgrading,
						Conditions: upgradev1alpha1.Conditions{
							upgradev1alpha1.UpgradeCondition{
								Type:      upgradev1alpha1.SendStartedNotification,
								StartTime: &metav1.Time{Time: testTime},
							},
						},
						StartTime:          &metav1.Time{Time: testTime},
						CompleteTime:       &metav1.Time{Time: testTime},
						WorkerStartTime:    &metav1.Time{Time: testTime},
						WorkerCompleteTime: &metav1.Time{Time: testTime},
					},
				},
			},
		}
		cv = configv1.ClusterVersion{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{Version: TEST_UPGRADE_VERSION},
				Channel:       TEST_UPGRADE_CHANNEL,
			},
			Status: configv1.ClusterVersionStatus{
				History: []configv1.UpdateHistory{
					{State: configv1.CompletedUpdate, Version: "something"},
				},
			},
		}
	})

	JustBeforeEach(func() {
		upgradeCollector = &UpgradeCollector{
			upgradeConfigManager: mockUpgradeConfigManager,
			cvClient:             mockCVClient,
			managedMetrics:       bootstrapMetrics(),
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("Collecting metrics of an upgrade", func() {
		Context("When UpgradeConfig is not found", func() {
			It("no metrics will be collected", func() {
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(nil, upgradeconfigmanager.ErrUpgradeConfigNotFound),
				)
				metricCount := promtestutil.CollectAndCount(upgradeCollector)
				Expect(metricCount).To(BeZero())
			})
			Context("When UpgradeConfig is found", func() {
				It("collects metrics based on availability of conditions", func() {
					gomock.InOrder(
						mockUpgradeConfigManager.EXPECT().Get().Return(&upgradeConfig, nil),
						mockCVClient.EXPECT().GetClusterVersion().Return(&cv, nil),
					)
					metricCount := promtestutil.CollectAndCount(upgradeCollector)
					Expect(metricCount).NotTo(BeZero())
				})
			})
		})
	})
})
