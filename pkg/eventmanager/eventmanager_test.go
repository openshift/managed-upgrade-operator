package eventmanager

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/types"

	"github.com/golang/mock/gomock"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	configMock "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	metricsMock "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	notifierMock "github.com/openshift/managed-upgrade-operator/pkg/notifier/mocks"
	ucMgrMock "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_OPERATOR_NAMESPACE = "openshift-managed-upgrade-operator"
	TEST_UPGRADECONFIG_CR   = "managed-upgrade-config"
	TEST_UPGRADE_VERSION    = "4.4.4"
	TEST_UPGRADE_TIME       = "2020-06-20T00:00:00Z"
)

var _ = Describe("OCM Notifier", func() {
	var (
		mockCtrl                 *gomock.Controller
		mockKubeClient           *mocks.MockClient
		mockUpgradeConfigManager *ucMgrMock.MockUpgradeConfigManager
		mockConfigManagerBuilder *configMock.MockConfigManagerBuilder
		mockNotifier             *notifierMock.MockNotifier
		mockMetricsClient        *metricsMock.MockMetrics
		manager                  *eventManager
		upgradeConfigName        types.NamespacedName
	)

	BeforeEach(func() {
		_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockUpgradeConfigManager = ucMgrMock.NewMockUpgradeConfigManager(mockCtrl)
		mockConfigManagerBuilder = configMock.NewMockConfigManagerBuilder(mockCtrl)
		mockNotifier = notifierMock.NewMockNotifier(mockCtrl)
		mockMetricsClient = metricsMock.NewMockMetrics(mockCtrl)
	})

	JustBeforeEach(func() {
		manager = &eventManager{
			client:               mockKubeClient,
			upgradeConfigManager: mockUpgradeConfigManager,
			notifier:             mockNotifier,
			metrics:              mockMetricsClient,
			configManagerBuilder: mockConfigManagerBuilder,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When notifying a completed state", func() {
		var uc upgradev1alpha1.UpgradeConfig
		var testState = notifier.StateCompleted
		BeforeEach(func() {
			upgradeConfigName = types.NamespacedName{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			}
			uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseUpgraded).GetUpgradeConfig()
			uc.Spec.Desired.Version = TEST_UPGRADE_VERSION
			uc.Status.History[0].Version = TEST_UPGRADE_VERSION
			uc.Spec.UpgradeAt = TEST_UPGRADE_TIME
		})

		Context("when a notification has already been sent", func() {
			It("does no action", func() {
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(true, nil),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})
		Context("when a notification has not been sent", func() {
			It("sends a correct notification", func() {
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, gomock.Any()),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})
		Context("when a notification can't be sent", func() {
			var fakeError = fmt.Errorf("fake error")
			It("returns an error", func() {
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, gomock.Any()).Return(fakeError),
				)
				err := manager.Notify(testState)
				Expect(err).NotTo(BeNil())
			})
		})

	})

	Context("When notifying a failed state", func() {
		var uc upgradev1alpha1.UpgradeConfig
		var testState = notifier.StateFailed
		BeforeEach(func() {
			upgradeConfigName = types.NamespacedName{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			}
			uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseUpgrading).GetUpgradeConfig()
			uc.Spec.Desired.Version = TEST_UPGRADE_VERSION
			uc.Status.History[0].Version = TEST_UPGRADE_VERSION
			uc.Spec.UpgradeAt = TEST_UPGRADE_TIME
		})

		Context("when the pre-health-check failed", func() {
			It("sends a correct notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.UpgradePreHealthCheck,
						Status:  "False",
						Reason:  "PreHealthCheck not done",
						Message: "There are 2 critical alerts",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_PREHEALTHCHECK_FAILED_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

		Context("when the external dependency check failed", func() {
			It("sends a correct notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.ExtDepAvailabilityCheck,
						Status:  "False",
						Reason:  "ExtDepAvailabilityCheck not done",
						Message: "An external dependency is down.",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_EXTDEPCHECK_FAILED_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

		Context("when the scale up fails", func() {
			It("sends a correct notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.UpgradeScaleUpExtraNodes,
						Status:  "False",
						Reason:  "UpgradeScaleUpExtraNodes not done",
						Message: "Cannot scale nodes.",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_SCALE_FAILED_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

		Context("when an indeterminate failure occurs", func() {
			It("sends a correct default notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.CommenceUpgrade,
						Status:  "False",
						Reason:  "something strange",
						Message: "in your neighbourhood",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_PRECHECK_FAILED_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

	})

	Context("When notifying a delayed state", func() {
		var uc upgradev1alpha1.UpgradeConfig
		var testState = notifier.StateDelayed
		BeforeEach(func() {
			upgradeConfigName = types.NamespacedName{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			}
			uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseUpgrading).GetUpgradeConfig()
			uc.Spec.Desired.Version = TEST_UPGRADE_VERSION
			uc.Status.History[0].Version = TEST_UPGRADE_VERSION
			uc.Spec.UpgradeAt = TEST_UPGRADE_TIME
		})

		Context("when the pre-health-check failed", func() {
			It("sends a correct notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.UpgradePreHealthCheck,
						Status:  "False",
						Reason:  "PreHealthCheck not done",
						Message: "There are 2 critical alerts",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_PREHEALTHCHECK_DELAY_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

		Context("when the external dependency check failed", func() {
			It("sends a correct notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.ExtDepAvailabilityCheck,
						Status:  "False",
						Reason:  "ExtDepAvailabilityCheck not done",
						Message: "An external dependency is down.",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_EXTDEPCHECK_DELAY_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

		Context("when the scale up fails", func() {
			It("sends a correct notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.UpgradeScaleUpExtraNodes,
						Status:  "False",
						Reason:  "UpgradeScaleUpExtraNodes not done",
						Message: "Cannot scale nodes.",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_SCALE_DELAY_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

		Context("when an indeterminate failure occurs", func() {
			It("sends a correct default notification and description", func() {
				uc.Status.History[0].Conditions = []upgradev1alpha1.UpgradeCondition{
					{
						Type:    upgradev1alpha1.CommenceUpgrade,
						Status:  "False",
						Reason:  "something strange",
						Message: "in your neighbourhood",
					},
				}
				expectedDescription := fmt.Sprintf(UPGRADE_DEFAULT_DELAY_DESC, uc.Spec.Desired.Version)
				gomock.InOrder(
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockMetricsClient.EXPECT().IsMetricNotificationEventSentSet(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION).Return(false, nil),
					mockNotifier.EXPECT().NotifyState(testState, expectedDescription),
					mockMetricsClient.EXPECT().UpdateMetricNotificationEventSent(TEST_UPGRADECONFIG_CR, string(testState), TEST_UPGRADE_VERSION),
				)
				err := manager.Notify(testState)
				Expect(err).To(BeNil())
			})
		})

	})

})
