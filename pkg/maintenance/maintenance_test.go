package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/golang/mock/gomock"
	routev1 "github.com/openshift/api/route/v1"
	ammocks "github.com/openshift/managed-upgrade-operator/pkg/alertmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	amv2Models "github.com/prometheus/alertmanager/api/v2/models"
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
	Context("Build Alert Manager", func() {
		It("Build an Alert Manager Client and not return an error", func() {
			var ammb alertManagerMaintenanceBuilder
			mockAmRoute := &routev1.Route{}
			mockSecretList := &corev1.SecretList{}

			mockKubeClient.EXPECT().Get(context.TODO(), types.NamespacedName{Namespace: alertManagerNamespace, Name: alertManagerRouteName}, mockAmRoute)
			mockKubeClient.EXPECT().List(context.TODO(), mockSecretList, &client.ListOptions{Namespace: alertManagerNamespace})

			_, err := ammb.NewClient(mockKubeClient)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
