package maintenance

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-openapi/strfmt"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/managed-upgrade-operator/config"
	ammocks "github.com/openshift/managed-upgrade-operator/pkg/alertmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/k8sutil"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	amv2Models "github.com/prometheus/alertmanager/api/v2/models"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Alert Manager Maintenance Client", func() {
	var (
		mockCtrl              *gomock.Controller
		mockKubeClient        *mocks.MockClient
		silenceClient         *ammocks.MockAlertManagerSilencer
		maintenance           alertManagerMaintenance
		testComment                 = "test comment"
		testOperatorName            = "managed-upgrade-operator"
		testCreatedByOperator       = testOperatorName
		testCreatedByTest           = "Tester the Creator"
		testNow                     = strfmt.DateTime(time.Now().UTC())
		testEnd                     = strfmt.DateTime(time.Now().UTC().Add(90 * time.Minute))
		testVersion                 = "V-1.million.25"
		testWorkerCount       int32 = 5
		testNewWorkerCount    int32 = 4

		// Certificate Authority
		fakeCAMap = make(map[string]string)

		// Create test silence created by the operator
		testSilence = amv2Models.Silence{
			Comment:   &testComment,
			CreatedBy: &testCreatedByOperator,
			EndsAt:    &testEnd,
			Matchers:  createDefaultMatchers(),
			StartsAt:  &testNow,
		}

		testNoActiveSilences = []amv2Models.GettableSilence{}

		activeSilenceId      = "test-id"
		activeSilenceStatus  = amv2Models.SilenceStatusStateActive
		activeSilenceComment = fmt.Sprintf("Silence for OSD with %d worker node upgrade to version %s", testWorkerCount, testVersion)
		testActiveSilences   = []amv2Models.GettableSilence{
			{
				ID:     &activeSilenceId,
				Status: &amv2Models.SilenceStatus{State: &activeSilenceStatus},
				Silence: amv2Models.Silence{
					Comment:   &activeSilenceComment,
					CreatedBy: &testCreatedByOperator,
					EndsAt:    &testEnd,
					Matchers:  createDefaultMatchers(),
					StartsAt:  &testNow,
				},
			},
		}
		ignoredControlPlaneCriticals = []string{"ignoredAlertSRE"}
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		silenceClient = ammocks.NewMockAlertManagerSilencer(mockCtrl)
		maintenance = alertManagerMaintenance{client: silenceClient}
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		fakeCAMap[metrics.MonitoringConfigField] = `
-----BEGIN CERTIFICATE-----
MIIDmTCCAoECFC/TvMvmk3V9QdPG4bPDIE085uvHMA0GCSqGSIb3DQEBCwUAMIGI
MQswCQYDVQQGEwJBVTEMMAoGA1UECAwDUUxEMRAwDgYDVQQHDAdEb2dGaXNoMRcw
FQYDVQQKDA5UcnVzdE1lSW1hQ2VydDEKMAgGA1UECwwBMzEUMBIGA1UEAwwLQ2Vy
dCBDb2JhaW4xHjAcBgkqhkiG9w0BCQEWD2NlcnR5QHRocmVlLmxvbDAeFw0yMTA4
MDMwOTE0MDlaFw00NjAzMjUwOTE0MDlaMIGIMQswCQYDVQQGEwJBVTEMMAoGA1UE
CAwDUUxEMRAwDgYDVQQHDAdEb2dGaXNoMRcwFQYDVQQKDA5UcnVzdE1lSW1hQ2Vy
dDEKMAgGA1UECwwBMzEUMBIGA1UEAwwLQ2VydCBDb2JhaW4xHjAcBgkqhkiG9w0B
CQEWD2NlcnR5QHRocmVlLmxvbDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoC
ggEBAMuaiFMw3xVBI24xKlGAugnhEOwT0QhZkupwe3cBFasNmK85LBrNiBqBnbSi
euf4uQj24eq9ot8Hz+OYKgt4NE8GadaOVd2s3SbHdJw6iMgZBpHI9spgXDV+k6nr
g4Cd6fjORfJWzJWesg25hqbv7EYLNhL/O46ezR44fF2zXn+ktT0T7RKBzwF/q1ep
+yT2MmxBitN1QXvcOOcvGepgz059Ly98Il1WyhitCQfZBVWV+OLCrTLOck5maxVU
T02AFq3ck0PLddsjFL43vsBFt6vJd5q2EBIdpr7Jh8wKpLLRLSIlwaeXvgRQrOes
d+dHf230ifbyhwcK9kHZbs5RFBsCAwEAATANBgkqhkiG9w0BAQsFAAOCAQEAJWDm
1ckuG0ssC0UvVGXOuYI7g44K6GvyZvFDx+YjhradSHYJruPz0QPyhGpwQdLmGuMA
QXLZioEzgqxfAml7YXVOuXPipQIT0L52AtTlkhk+rtPEJUVK3uRcoj4glzJs9stk
fDOG8bfDQiZUpNa0Ohwphyz4L1BuOPPBuHShBI1Iwad5r/ZxGzcll2pUgA9IOjB4
RjP0ZHzlKcUJeESVpnPYrO4J5JxcTNEUJYW0zSTpuqEh7qiKZUaPf7c8KMsDco9T
O3zCGH3n3V74P27js1duFytlfoGhTscwcByl500lT9fXq1QVPuSNXQIzYsnYTf8e
SLKh9n6qnPj0Lef3Nw==
-----END CERTIFICATE-----
`
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	// Starting a Control Plane Silence
	Context("Creating a Control Plane silence", func() {
		It("Should not error on successful maintenance start", func() {
			gomock.InOrder(
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&testNoActiveSilences, nil).Times(2),
				silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2),
			)
			end := time.Now().Add(90 * time.Minute)
			err := maintenance.StartControlPlane(end, testVersion, ignoredControlPlaneCriticals)
			Expect(err).Should(Not(HaveOccurred()))
		})
		It("Should error on failing to start maintenance", func() {
			gomock.InOrder(
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&testNoActiveSilences, nil).Times(2),
				silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error")),
			)
			end := time.Now().Add(90 * time.Minute)
			err := maintenance.StartControlPlane(end, testVersion, ignoredControlPlaneCriticals)
			Expect(err).Should(HaveOccurred())
		})
	})

	// Starting a worker silence
	Context("Creating a worker silence", func() {
		It("Should not error on successful maintenance start", func() {
			gomock.InOrder(
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&testNoActiveSilences, nil).Times(2),
				silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
			)
			end := time.Now().Add(90 * time.Minute)
			err := maintenance.SetWorker(end, testVersion, testWorkerCount)
			Expect(err).Should(Not(HaveOccurred()))
		})
		It("Should error on failing to start maintenance", func() {
			gomock.InOrder(
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&testNoActiveSilences, nil).Times(2),
				silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error")),
			)
			end := time.Now().Add(90 * time.Minute)
			err := maintenance.SetWorker(end, testVersion, testWorkerCount)
			Expect(err).Should(HaveOccurred())
		})
	})

	// Do not update if worker count unchanged
	Context("Do not create new silence", func() {
		It("Should not create new silence if one already exists with same comment", func() {
			gomock.InOrder(
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&testActiveSilences, nil),
			)
			end := time.Now().Add(90 * time.Minute)
			err := maintenance.SetWorker(end, testVersion, testWorkerCount)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	// Delete old silence and create new one with new worker count
	Context("Recreate silence", func() {
		It("Should create new silence when the worker count changed", func() {
			gomock.InOrder(
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&testNoActiveSilences, nil),
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&testActiveSilences, nil),
				silenceClient.EXPECT().Delete(gomock.Any()),
				silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
			)
			end := time.Now().Add(90 * time.Minute)
			err := maintenance.SetWorker(end, testVersion, testNewWorkerCount)
			Expect(err).ShouldNot(HaveOccurred())
		})

	})

	// Finding and removing all active maintenances
	Context("End all active maintenances/silences", func() {
		// Create test vars for retrieving test maintenance objects
		testId := "testId"
		testUpdatedAt := strfmt.DateTime(time.Now().UTC())
		testState := "active"
		testStatus := amv2Models.SilenceStatus{
			State: &testState,
		}

		It("Should not error if no maintenances are found", func() {
			testSilenceNotOwned := testSilence
			testSilenceNotOwned.CreatedBy = &testCreatedByTest

			silenceClient.EXPECT().Filter(gomock.Any()).Return(&[]amv2Models.GettableSilence{}, nil)
			err := maintenance.EndSilences("")
			Expect(err).Should(Not(HaveOccurred()))
		})
		It("Should find maintenances created by the operator and not return an error", func() {
			testSilence.CreatedBy = &testCreatedByOperator

			// Create mock GettableSilence object to return
			gettableSilence := amv2Models.GettableSilence{
				ID:        &testId,
				Status:    &testStatus,
				UpdatedAt: &testUpdatedAt,
				Silence:   testSilence,
			}

			// Append GettableSilence to GettableSilences
			var activeSilences []amv2Models.GettableSilence
			activeSilences = append(activeSilences, gettableSilence)

			gomock.InOrder(
				silenceClient.EXPECT().Filter(gomock.Any()).Return(&activeSilences, nil),
				silenceClient.EXPECT().Delete(testId).Return(nil),
			)
			err := maintenance.EndSilences("")
			Expect(err).Should(Not(HaveOccurred()))
		})
	})
	// Finding and removing all active maintenances
	Context("Build Alert Manager in production", func() {
		It("Build an Alert Manager Client and not return an error", func() {
			var ammb alertManagerMaintenanceBuilder
			mockSecretList := &corev1.SecretList{}
			mockAmService := &corev1.Service{}
			mockMonConfigMap := &corev1.ConfigMap{}
			fakeMonConfigMap := &corev1.ConfigMap{
				Data: fakeCAMap,
			}

			mockKubeClient.EXPECT().Get(context.TODO(), types.NamespacedName{Namespace: metrics.MonitoringNS, Name: alertManagerApp}, mockAmService)
			mockKubeClient.EXPECT().List(context.TODO(), mockSecretList, &client.ListOptions{Namespace: metrics.MonitoringNS})
			mockKubeClient.EXPECT().Get(context.TODO(), client.ObjectKey{Name: metrics.MonitoringCAConfigMapName, Namespace: metrics.MonitoringNS}, mockMonConfigMap).SetArg(2, *fakeMonConfigMap).Return(nil)

			_, err := ammb.NewClient(mockKubeClient)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
	Context("Build Alert Manager for local development replicating production by using Services", func() {
		It("Build an Alert Manager Client and not return an error", func() {
			_ = os.Setenv(k8sutil.ForceRunModeEnv, string(k8sutil.LocalRunMode))
			var ammb alertManagerMaintenanceBuilder
			mockAmService := &corev1.Service{}
			mockSecretList := &corev1.SecretList{}
			mockMonConfigMap := &corev1.ConfigMap{}
			fakeMonConfigMap := &corev1.ConfigMap{
				Data: fakeCAMap,
			}

			mockKubeClient.EXPECT().Get(context.TODO(), types.NamespacedName{Namespace: metrics.MonitoringNS, Name: alertManagerApp}, mockAmService)
			mockKubeClient.EXPECT().List(context.TODO(), mockSecretList, &client.ListOptions{Namespace: metrics.MonitoringNS})
			mockKubeClient.EXPECT().Get(context.TODO(), client.ObjectKey{Name: metrics.MonitoringCAConfigMapName, Namespace: metrics.MonitoringNS}, mockMonConfigMap).SetArg(2, *fakeMonConfigMap).Return(nil)

			_, err := ammb.NewClient(mockKubeClient)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
	Context("Build Alert Manager for local development not replicating production by using Routes", func() {
		It("Build an Alert Manager Client and not return an error", func() {
			_ = os.Setenv(k8sutil.ForceRunModeEnv, string(k8sutil.LocalRunMode))
			_ = os.Setenv(config.EnvRoutes, "true")
			var ammb alertManagerMaintenanceBuilder
			mockAmRoute := &routev1.Route{}
			mockSecretList := &corev1.SecretList{}

			mockKubeClient.EXPECT().Get(context.TODO(), types.NamespacedName{Namespace: metrics.MonitoringNS, Name: alertManagerApp}, mockAmRoute)
			mockKubeClient.EXPECT().List(context.TODO(), mockSecretList, &client.ListOptions{Namespace: metrics.MonitoringNS})

			_, err := ammb.NewClient(mockKubeClient)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
