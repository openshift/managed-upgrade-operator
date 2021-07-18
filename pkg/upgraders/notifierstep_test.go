package upgraders

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("NotifierStep", func() {
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

	Context("When running the send-started-notification phase", func() {
		Context("When the cluster is upgrading", func() {
			It("will return without doing anything", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil),
				)
				result, err := upgrader.SendStartedNotification(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When the cluster has not started upgrading yet", func() {
			It("will send the correct notification", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockEMClient.EXPECT().Notify(notifier.MuoStateStarted),
				)
				result, err := upgrader.SendStartedNotification(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will not succeed if it can't send the notification", func() {
				fakeErr := fmt.Errorf("fake error")
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockEMClient.EXPECT().Notify(notifier.MuoStateStarted).Return(fakeErr),
				)
				result, err := upgrader.SendStartedNotification(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("When running the send-completed-notification phase", func() {
		It("will send the notification", func() {
			gomock.InOrder(
				mockEMClient.EXPECT().Notify(notifier.MuoStateCompleted),
			)
			result, err := upgrader.SendCompletedNotification(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("will not succeed if it can't send the notification", func() {
			fakeErr := fmt.Errorf("fake error")
			gomock.InOrder(
				mockEMClient.EXPECT().Notify(notifier.MuoStateCompleted).Return(fakeErr),
			)
			result, err := upgrader.SendCompletedNotification(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})

	Context("When running the send-delayed-notification phase", func() {
		Context("when the upgrade hasn't yet started", func() {
			Context("when the upgrade is delayed", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
						{
							Version:   upgradeConfig.Spec.Desired.Version,
							Phase:     upgradev1alpha1.UpgradePhaseUpgrading,
							StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
						},
					}
				})
				Context("when the delay trigger is 0", func() {
					BeforeEach(func() {
						config.UpgradeWindow.DelayTrigger = 0
					})
					It("will not notify as delayed", func() {
						gomock.InOrder(
							mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
						)
						result, err := upgrader.UpgradeDelayedCheck(context.TODO(), logger)
						Expect(err).NotTo(HaveOccurred())
						Expect(result).To(BeTrue())
					})
				})
				It("will send a notification", func() {
					gomock.InOrder(
						mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
						mockEMClient.EXPECT().Notify(notifier.MuoStateDelayed).Return(nil),
					)
					result, err := upgrader.UpgradeDelayedCheck(context.TODO(), logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeTrue())
				})
				It("will fail if a notification can't be sent", func() {
					fakeError := fmt.Errorf("fake error")
					gomock.InOrder(
						mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
						mockEMClient.EXPECT().Notify(notifier.MuoStateDelayed).Return(fakeError),
					)
					result, err := upgrader.UpgradeDelayedCheck(context.TODO(), logger)
					Expect(err).To(HaveOccurred())
					Expect(result).To(BeFalse())
				})
			})
			Context("when the upgrade is not delayed", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
						{
							Version:   upgradeConfig.Spec.Desired.Version,
							Phase:     upgradev1alpha1.UpgradePhaseUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
						},
					}
				})
				It("will not send a notification", func() {
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil)
					result, err := upgrader.UpgradeDelayedCheck(context.TODO(), logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeTrue())
				})
			})
		})
		Context("when the upgrade has started", func() {
			BeforeEach(func() {
				upgradeConfig.Spec.UpgradeAt = time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
			})
			It("will not send a notification", func() {
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil)
				result, err := upgrader.UpgradeDelayedCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
	})
})
