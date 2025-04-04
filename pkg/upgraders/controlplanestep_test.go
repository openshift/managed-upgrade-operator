package upgraders

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	gomock "go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("ControlPlaneStep", func() {
	var (
		logger logr.Logger
		// mocks
		mockKubeClient           *mocks.MockClient
		mockCtrl                 *gomock.Controller
		mockMaintClient          *mockMaintenance.MockMaintenance
		mockScalerClient         *mockScaler.MockScaler
		mockMachineryClient      *mockMachinery.MockMachinery
		mockMetricsClient        *mockMetrics.MockMetrics
		mockCVClient             *cvMocks.MockClusterVersion
		mockDrainStrategyBuilder *mockDrain.MockNodeDrainStrategyBuilder
		mockEMClient             *emMocks.MockEventManager
		// upgradeconfig to be used during tests
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig

		// upgrader to be used during tests
		config   *upgraderConfig
		upgrader *clusterUpgrader
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockMaintClient = mockMaintenance.NewMockMaintenance(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockScalerClient = mockScaler.NewMockScaler(mockCtrl)
		mockMachineryClient = mockMachinery.NewMockMachinery(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		mockDrainStrategyBuilder = mockDrain.NewMockNodeDrainStrategyBuilder(mockCtrl)
		mockEMClient = emMocks.NewMockEventManager(mockCtrl)
		logger = logf.Log.WithName("cluster upgrader test logger")
		config = buildTestUpgraderConfig(90, 30, 8, 120, 30)
		upgrader = &clusterUpgrader{
			client:               mockKubeClient,
			metrics:              mockMetricsClient,
			cvClient:             mockCVClient,
			notifier:             mockEMClient,
			config:               config,
			scaler:               mockScalerClient,
			drainstrategyBuilder: mockDrainStrategyBuilder,
			maintenance:          mockMaintClient,
			machinery:            mockMachineryClient,
			upgradeConfig:        upgradeConfig,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When assessing if the control plane is upgraded to a version", func() {
		Context("When the clusterversion can't be fetched", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				mockCVClient.EXPECT().GetClusterVersion().Return(nil, fakeError)
				result, err := upgrader.ControlPlaneUpgraded(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})

		Context("When cannot notify control plane upgrade finished", func() {
			It("Should report error", func() {
				fakeError := fmt.Errorf("fake notification error")
				gomock.InOrder(
					mockCVClient.EXPECT().GetClusterVersion().Return(nil, nil),
					mockCVClient.EXPECT().HasUpgradeCompleted(gomock.Any(), gomock.Any()).Return(true),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(fakeError),
				)
				result, err := upgrader.ControlPlaneUpgraded(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When that version is recorded in clusterversion's history", func() {
			var clusterVersion *configv1.ClusterVersion
			BeforeEach(func() {
				clusterVersion = &configv1.ClusterVersion{
					Status: configv1.ClusterVersionStatus{
						History: []configv1.UpdateHistory{
							{State: configv1.CompletedUpdate, Version: "something"},
							{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version, StartedTime: metav1.Time{Time: time.Now().Add(-time.Duration(-10 * time.Minute))}, CompletionTime: &metav1.Time{Time: time.Now()}},
							{State: configv1.CompletedUpdate, Version: "something else"},
						},
					},
				}
			})
			It("Flags the control plane as upgraded", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
					mockCVClient.EXPECT().HasUpgradeCompleted(gomock.Any(), gomock.Any()).Return(true),
					mockEMClient.EXPECT().Notify(gomock.Any()),
					mockMetricsClient.EXPECT().ResetMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockCVClient.EXPECT().GetClusterId(),
					mockMetricsClient.EXPECT().UpdateMetricControlplaneUpgradeCompletedTimestamp(gomock.Any(), upgradeConfig.Name, gomock.Any(), gomock.Any()),
					mockMetricsClient.EXPECT().UpdateMetricWorkernodeUpgradeStartedTimestamp(gomock.Any(), upgradeConfig.Name, gomock.Any(), gomock.Any()),
				)
				result, err := upgrader.ControlPlaneUpgraded(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

		Context("When that version is not recorded in clusterversion's history", func() {
			var clusterVersion *configv1.ClusterVersion
			BeforeEach(func() {
				clusterVersion = &configv1.ClusterVersion{
					Status: configv1.ClusterVersionStatus{
						History: []configv1.UpdateHistory{
							{State: configv1.CompletedUpdate, Version: "something"},
							{State: configv1.CompletedUpdate, Version: "something else"},
						},
					},
				}
			})

			Context("When the upgrade window has expired", func() {
				BeforeEach(func() {
					upgradeConfig.Spec.UpgradeAt = time.Now().Add(-300 * time.Minute).Format(time.RFC3339)
				})
				It("Sets the upgrade timeout flag", func() {
					gomock.InOrder(
						mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
						mockCVClient.EXPECT().HasUpgradeCompleted(gomock.Any(), gomock.Any()).Return(false),
						mockMetricsClient.EXPECT().UpdateMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					)
					result, err := upgrader.ControlPlaneUpgraded(context.TODO(), logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeFalse())
				})
			})

			Context("When the upgrade window has not yet expired", func() {
				BeforeEach(func() {
					upgradeConfig.Spec.UpgradeAt = time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
				})
				It("Does not set the upgrade timeout flag", func() {
					gomock.InOrder(
						mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
						mockCVClient.EXPECT().HasUpgradeCompleted(gomock.Any(), gomock.Any()).Return(false),
					)
					result, err := upgrader.ControlPlaneUpgraded(context.TODO(), logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeFalse())
				})
			})
		})

		Context("When the control plane hasn't upgraded within the window", func() {
			var clusterVersion *configv1.ClusterVersion
			upgradeStartTime := time.Now().Add(-300 * time.Minute)
			BeforeEach(func() {
				clusterVersion = &configv1.ClusterVersion{
					Status: configv1.ClusterVersionStatus{
						History: []configv1.UpdateHistory{
							{State: configv1.PartialUpdate, Version: upgradeConfig.Spec.Desired.Version, StartedTime: metav1.Time{Time: upgradeStartTime}},
						},
					},
				}
			})
			It("Sets the appropriate metric", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
					mockCVClient.EXPECT().HasUpgradeCompleted(gomock.Any(), gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
				)
				result, err := upgrader.ControlPlaneUpgraded(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("When requesting the cluster to begin upgrading", func() {
		Context("When the clusterversion version can't be fetched", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("a fake error")
				gomock.InOrder(
					mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()),
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, fakeError),
				)
				result, err := upgrader.CommenceUpgrade(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})

		Context("When setting the desired version fails", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				gomock.InOrder(
					mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()),
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockEMClient.EXPECT().Notify(gomock.Any()),
					mockCVClient.EXPECT().GetClusterId(),
					mockMetricsClient.EXPECT().UpdateMetricControlplaneUpgradeStartedTimestamp(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()),
					mockCVClient.EXPECT().EnsureDesiredConfig(gomock.Any()).Return(false, fakeError),
				)
				result, err := upgrader.CommenceUpgrade(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})

		Context("When sending notification for control plane upgrade start fails", func() {
			It("Should report error", func() {
				fakeError := fmt.Errorf("fake notification error")
				gomock.InOrder(
					mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()),
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(fakeError),
				)
				result, err := upgrader.CommenceUpgrade(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When clusterversion is upgraded to desired version", func() {
			It("Should return the control plane upgrade completion with no error", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()),
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockEMClient.EXPECT().Notify(gomock.Any()),
					mockCVClient.EXPECT().GetClusterId(),
					mockMetricsClient.EXPECT().UpdateMetricControlplaneUpgradeStartedTimestamp(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()),
					mockCVClient.EXPECT().EnsureDesiredConfig(gomock.Any()).Return(true, nil),
				)
				result, err := upgrader.CommenceUpgrade(context.TODO(), logger)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})

		})
	})

	Context("When the cluster's upgrade process has commenced", func() {
		It("will not re-perform commencing an upgrade", func() {
			gomock.InOrder(
				mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()),
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil),
			)
			result, err := upgrader.CommenceUpgrade(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

	Context("When the upgrader can't tell if the cluster's upgrade has commenced", func() {
		var fakeError = fmt.Errorf("fake upgradeCommenced error")
		It("will abort the commencing of an upgrade", func() {
			gomock.InOrder(
				mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()),
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, fakeError),
			)
			result, err := upgrader.CommenceUpgrade(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeError))
			Expect(result).To(BeFalse())
		})
	})

})
