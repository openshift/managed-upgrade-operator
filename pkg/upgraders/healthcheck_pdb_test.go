package upgraders

import (
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	dvoMocks "github.com/openshift/managed-upgrade-operator/pkg/dvo/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	"go.uber.org/mock/gomock"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("checkPodDisruptionBudgets", func() {
	var (
		mockClient        *mocks.MockClient
		mockCtrl          *gomock.Controller
		logger            logr.Logger
		mockMetricsClient *mockMetrics.MockMetrics
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
		mockdvoclientbulder     		*dvoMocks.MockDvoClientBuilder
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockdvoclientbulder = dvoMocks.NewMockDvoClientBuilder(mockCtrl)
		logger = logf.Log.WithName("cluster upgrader test logger")
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
	})

	Context("When there is invalid PDB configuration", func() {
		It("HealthCheckPDB check will fail", func() {
			pdbList := &policyv1.PodDisruptionBudgetList{
				Items: []policyv1.PodDisruptionBudget{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-logging",
							Name:      "pdb-1",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MaxUnavailable: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: 0,
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-operators",
							Name:      "pdb-2",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MinAvailable: &intstr.IntOrString{
								Type:   intstr.String,
								StrVal: "100%",
							},
						},
					},
				},
			}
			gomock.InOrder(
				mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *pdbList),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, metrics.ClusterInvalidPDBConf),
			)
			reason, err := HealthCheckPDB(mockMetricsClient, mockClient, mockdvoclientbulder, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(reason).To(Equal(false))
		})
	})

	Context("When no invalid PDB", func() {
		It("Prehealth PDB check will pass", func() {
			pdbList := &policyv1.PodDisruptionBudgetList{
				Items: []policyv1.PodDisruptionBudget{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-logging",
							Name:      "pdb-1",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MaxUnavailable: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: 1,
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-operators",
							Name:      "pdb-2",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MinAvailable: &intstr.IntOrString{
								Type:   intstr.String,
								StrVal: "50%",
							},
						},
					},
				},
			}
			gomock.InOrder(
				mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *pdbList),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, metrics.ClusterInvalidPDB),
			)
			reason, err := checkPodDisruptionBudgets(mockClient, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(BeEmpty())
		})
	})

	Context("When there is invalid PDB configuration", func() {
		It("Prehealth PDB check will fail", func() {
			pdbList := &policyv1.PodDisruptionBudgetList{
				Items: []policyv1.PodDisruptionBudget{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-logging",
							Name:      "pdb-1",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MaxUnavailable: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: 0,
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-operators",
							Name:      "pdb-2",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MinAvailable: &intstr.IntOrString{
								Type:   intstr.String,
								StrVal: "100%",
							},
						},
					},
				},
			}
			gomock.InOrder(
				mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *pdbList),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, metrics.ClusterInvalidPDBConf),
			)
			reason, err := checkPodDisruptionBudgets(mockClient, logger)
			Expect(err).To(HaveOccurred())
			Expect(reason).To(Equal(metrics.ClusterInvalidPDBConf))
		})
	})

	Context("When there is invalid PDB configuration with incorrect minavailable", func() {
		It("Prehealth PDB check will fail", func() {
			pdbList := &policyv1.PodDisruptionBudgetList{
				Items: []policyv1.PodDisruptionBudget{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "openshift-operators",
							Name:      "pdb-2",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MinAvailable: &intstr.IntOrString{
								Type:   intstr.String,
								StrVal: "100%",
							},
						},
					},
				},
			}
			gomock.InOrder(
				mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *pdbList),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, metrics.ClusterInvalidPDBConf),
			)
			reason, err := checkPodDisruptionBudgets(mockClient, logger)
			Expect(err).To(HaveOccurred())
			Expect(reason).To(Equal(metrics.ClusterInvalidPDBConf))
		})
	})

	Context("When there is invalid PDB configuration with PDBQueryFailed", func() {
		It("Prehealth PDB check will fail", func() {

			gomock.InOrder(
				mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("Fake cannot fetch all pdb ")),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, metrics.PDBQueryFailed),
			)
			reason, err := checkPodDisruptionBudgets(mockClient, logger)
			Expect(err).To(HaveOccurred())
			Expect(reason).To(Equal(metrics.PDBQueryFailed))
		})
	})

	Context("checkNamespaceExistsInArray", func() {
		It("checkNamespaceExistsInArray fail", func() {

			result := checkNamespaceExistsInArray(namespaceException, "openshift-logging-1")
			Expect(result).To(Equal(false))
		})
	})

	Context("checkNamespaceExistsInArray", func() {
		It("checkNamespaceExistsInArray fail", func() {

			result := checkNamespaceExistsInArray(namespaceException, "openshift-logging")
			Expect(result).To(Equal(true))
		})
	})
})
