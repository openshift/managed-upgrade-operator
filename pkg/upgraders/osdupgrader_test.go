package upgraders

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	metricsMocks "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	scalerMocks "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradesteps"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("OSD upgrade failure policy", func() {
	var (
		mockCtrl      *gomock.Controller
		cvClient      *cvMocks.MockClusterVersion
		upgradeConfig *upgradev1alpha1.UpgradeConfig
		upgrader      *osdUpgrader
		stepRan       bool
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cvClient = cvMocks.NewMockClusterVersion(mockCtrl)
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().GetUpgradeConfig()
		upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{{
			Version:   upgradeConfig.Spec.Desired.Version,
			Phase:     upgradev1alpha1.UpgradePhaseUpgrading,
			StartTime: &metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
		}}
		stepRan = false
		upgrader = &osdUpgrader{clusterUpgrader: &clusterUpgrader{
			cvClient: cvClient,
			config:   buildTestUpgraderConfig(90, 30, 8, 120, 30),
			steps: []upgradesteps.UpgradeStep{
				upgradesteps.Action("test-step", func(context.Context, logr.Logger) (bool, error) {
					stepRan = true
					return true, nil
				}),
			},
		}}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	for _, readErr := range []error{fmt.Errorf("ClusterVersion read failed"), context.Canceled, context.DeadlineExceeded} {
		It("stops before upgrade steps when the policy read returns "+readErr.Error(), func() {
			ctx := context.Background()
			before := upgradeConfig.DeepCopy()
			cvClient.EXPECT().HasUpgradeCommenced(ctx, upgradeConfig).Return(false, readErr)

			phase, err := upgrader.UpgradeCluster(ctx, upgradeConfig, logr.Discard())

			Expect(err).To(BeIdenticalTo(readErr), "the original policy read error must reach reconciliation")
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgrading), "a failed policy read must leave the upgrade retryable")
			Expect(stepRan).To(BeFalse(), "upgrade steps must not run before the failure policy is determined")
			Expect(upgradeConfig).To(Equal(before), "a failed policy read must not change upgrade history")
		})
	}

	It("runs upgrade steps when the upgrade has already commenced", func() {
		cvClient.EXPECT().HasUpgradeCommenced(gomock.Any(), upgradeConfig).Return(true, nil)

		phase, err := upgrader.UpgradeCluster(context.Background(), upgradeConfig, logr.Discard())

		Expect(err).NotTo(HaveOccurred(), "a successful policy read should allow an ongoing upgrade")
		Expect(stepRan).To(BeTrue(), "an already commenced upgrade must continue despite the expired window")
		Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgraded), "successful steps should complete the upgrade")
	})

	It("preserves failure handling when the upgrade window has expired", func() {
		scaler := scalerMocks.NewMockScaler(mockCtrl)
		events := emMocks.NewMockEventManager(mockCtrl)
		metrics := metricsMocks.NewMockMetrics(mockCtrl)
		upgrader.scaler, upgrader.notifier, upgrader.metrics = scaler, events, metrics
		gomock.InOrder(
			cvClient.EXPECT().HasUpgradeCommenced(gomock.Any(), upgradeConfig).Return(false, nil),
			scaler.EXPECT().EnsureScaleDownNodes(gomock.Any(), gomock.Nil(), gomock.Any()).Return(true, nil),
			events.EXPECT().Notify(notifier.MuoStateFailed).Return(nil),
			metrics.EXPECT().UpdateMetricUpgradeWindowBreached(upgradeConfig.Name),
			metrics.EXPECT().ResetFailureMetrics(),
		)

		phase, err := upgrader.UpgradeCluster(context.Background(), upgradeConfig, logr.Discard())

		Expect(err).NotTo(HaveOccurred(), "the existing failure cleanup should succeed")
		Expect(stepRan).To(BeFalse(), "an expired upgrade must not execute upgrade steps")
		Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseFailed), "the expired upgrade should retain the failed phase")
	})
})
