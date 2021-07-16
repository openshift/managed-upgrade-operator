package upgradesteps

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
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
	)

	BeforeEach(func() {
		logger = logf.Log.WithName("step runner test logger")
	})

	Context("When all steps return successfully", func() {
		finalStepName := "final step"
		steps := []UpgradeStep{
			Action("step 1", successfulStep),
			Action("step 2", successfulStep),
			Action(finalStepName, successfulStep),
		}
		It("should return an upgrade completed phase", func() {
			phase, _, err := Run(context.TODO(), logger, steps)
			Expect(err).To(BeNil())
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgraded))
		})
		It("should indicate which step was the final condition to be satisfied", func() {
			phase, condition, err := Run(context.TODO(), logger, steps)
			Expect(err).To(BeNil())
			Expect(condition.Status).To(Equal(corev1.ConditionTrue))
			Expect(condition.Type).To(Equal(upgradev1alpha1.UpgradeConditionType(finalStepName)))
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgraded))
		})
	})

	Context("When a step is unsuccessful", func() {
		unsuccessfulStepName := "step that did not finish"
		steps := []UpgradeStep{
			Action("step 1", successfulStep),
			Action(unsuccessfulStepName, unsuccessfulStep),
			Action("step 3", successfulStep),
		}

		It("should indicate the upgrade is still ongoing", func() {
			phase, _, err := Run(context.TODO(), logger, steps)
			Expect(err).To(BeNil())
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgrading))
		})
		It("should indicate the upgrade condition which has not passed", func() {
			_, condition, err := Run(context.TODO(), logger, steps)
			Expect(err).To(BeNil())
			Expect(condition.Status).To(Equal(corev1.ConditionFalse))
			Expect(condition.Type).To(Equal(upgradev1alpha1.UpgradeConditionType(unsuccessfulStepName)))
		})
	})

	Context("When a step has errored", func() {
		erroredStepName := "step that errored"
		steps := []UpgradeStep{
			Action("step 1", successfulStep),
			Action(erroredStepName, erroredStep),
			Action("step 3", successfulStep),
		}

		It("should indicate the upgrade is still ongoing", func() {
			phase, _, _ := Run(context.TODO(), logger, steps)
			Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgrading))
		})
		It("should indicate the upgrade condition which has not passed", func() {
			_, condition, _ := Run(context.TODO(), logger, steps)
			Expect(condition.Status).To(Equal(corev1.ConditionFalse))
			Expect(condition.Type).To(Equal(upgradev1alpha1.UpgradeConditionType(erroredStepName)))
		})
		It("should indicate the error associated with the failed step", func() {
			_, _, err := Run(context.TODO(), logger, steps)
			Expect(err).To(Equal(err))
		})
	})
})
