package upgradesteps

import (
	"context"
	"fmt"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
)

var _ = Describe("HealthCheckStep", func() {

	var (
		logger logr.Logger

		successfulStep = func(ctx context.Context, logger logr.Logger) (bool, error) {
			return true, nil
		}
		erroredStep = func(ctx context.Context, logger logr.Logger) (bool, error) {
			return false, fmt.Errorf("a bad time")
		}
		unsuccessfulStep = func(ctx context.Context, logger logr.Logger) (bool, error) {
			return false, nil
		}
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
	)

	BeforeEach(func() {
		logger = logf.Log.WithName("step runner test logger")
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		upgradeConfig.Status.History.SetHistory(upgradev1alpha1.UpgradeHistory{
			Version: upgradeConfig.Spec.Desired.Version,
			Phase:   upgradev1alpha1.UpgradePhaseNew,
		})
	})

	Context("When all steps return successfully", func() {
		finalStepName := "final step"
		steps := []UpgradeStep{
			Action("step 1", successfulStep),
			Action("step 2", successfulStep),
			Action(finalStepName, successfulStep),
		}
		It("should return an upgrade completed phase", func() {
			phase, err := Run(context.TODO(), upgradeConfig, logger, steps)
			Expect(err).To(BeNil())
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgraded))
		})
		It("should have a successful condition for each step", func() {
			_, err := Run(context.TODO(), upgradeConfig, logger, steps)
			Expect(err).To(BeNil())
			history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
			Expect(history).ToNot(BeNil())
			for _, step := range steps {
				condition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(step.String()))
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(corev1.ConditionTrue))
				Expect(condition.CompleteTime).ToNot(BeNil())
			}
		})
	})

	Context("When a step is unsuccessful", func() {
		unsuccessfulStepName := "step that did not finish"
		successfulStepName := "step 1"
		notRunStepName := "step 3"
		steps := []UpgradeStep{
			Action(successfulStepName, successfulStep),
			Action(unsuccessfulStepName, unsuccessfulStep),
			Action(notRunStepName, successfulStep),
		}

		It("should indicate the upgrade is still ongoing", func() {
			phase, err := Run(context.TODO(), upgradeConfig, logger, steps)
			Expect(err).To(BeNil())
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgrading))
		})

		It("should correctly indicate condition states", func() {
			_, err := Run(context.TODO(), upgradeConfig, logger, steps)
			Expect(err).To(BeNil())
			history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
			Expect(history).ToNot(BeNil())
			successfulStepCondition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(successfulStepName))
			unsuccessfulStepCondition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(unsuccessfulStepName))
			missingStepCondition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(notRunStepName))
			Expect(missingStepCondition).To(BeNil())
			Expect(successfulStepCondition).ToNot(BeNil())
			Expect(successfulStepCondition.Status).To(Equal(corev1.ConditionTrue))
			Expect(successfulStepCondition.StartTime).ToNot(BeNil())
			Expect(successfulStepCondition.CompleteTime).ToNot(BeNil())
			Expect(unsuccessfulStepCondition).ToNot(BeNil())
			Expect(unsuccessfulStepCondition.Status).To(Equal(corev1.ConditionFalse))
			Expect(unsuccessfulStepCondition.StartTime).ToNot(BeNil())
			Expect(unsuccessfulStepCondition.CompleteTime).To(BeNil())
		})
	})

	Context("When a step has errored", func() {
		erroredStepName := "step that errored"
		successfulStepName := "step 1"
		notRunStepName := "step 3"
		steps := []UpgradeStep{
			Action(successfulStepName, successfulStep),
			Action(erroredStepName, erroredStep),
			Action(notRunStepName, successfulStep),
		}

		It("should indicate the upgrade is still ongoing", func() {
			phase, _ := Run(context.TODO(), upgradeConfig, logger, steps)
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgrading))
		})
		It("should indicate the error associated with the failed step", func() {
			_, err := Run(context.TODO(), upgradeConfig, logger, steps)
			Expect(err).To(Equal(err))
		})
		It("should correctly indicate condition states", func() {
			_, err := Run(context.TODO(), upgradeConfig, logger, steps)
			Expect(err).To(Equal(err))
			history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
			Expect(history).ToNot(BeNil())
			successfulStepCondition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(successfulStepName))
			erroredStepCondition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(erroredStepName))
			missingStepCondition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeConditionType(notRunStepName))
			Expect(missingStepCondition).To(BeNil())
			Expect(successfulStepCondition).ToNot(BeNil())
			Expect(successfulStepCondition.Status).To(Equal(corev1.ConditionTrue))
			Expect(successfulStepCondition.StartTime).ToNot(BeNil())
			Expect(successfulStepCondition.CompleteTime).ToNot(BeNil())
			Expect(erroredStepCondition).ToNot(BeNil())
			Expect(erroredStepCondition.Status).To(Equal(corev1.ConditionFalse))
			Expect(erroredStepCondition.StartTime).ToNot(BeNil())
			Expect(erroredStepCondition.CompleteTime).To(BeNil())
		})
	})
})
