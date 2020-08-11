package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	amSilence "github.com/prometheus/alertmanager/api/v2/client/silence"
	amv2Models "github.com/prometheus/alertmanager/api/v2/models"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Alert Manager Maintenance Client", func() {
	var (
		mockCtrl              *gomock.Controller
		mockKubeClient        *mocks.MockClient
		silenceClient         *MockAlertManagerSilencer
		testComment           = "test comment"
		testOperatorName      = "managed-upgrade-operator"
		testCreatedByOperator = testOperatorName
		testCreatedByTest     = "Tester the Creator"
		testNow               = strfmt.DateTime(time.Now().UTC())
		testEnd               = strfmt.DateTime(time.Now().UTC().Add(90 * time.Minute))
		testVersion           = "V-1.million.25"
		testGettableSilences  amv2Models.GettableSilences

		// Create test silence created by the operator
		testSilence = amv2Models.Silence{
			Comment:   &testComment,
			CreatedBy: &testCreatedByOperator,
			EndsAt:    &testEnd,
			Matchers:  createDefaultMatchers(),
			StartsAt:  &testNow,
		}

		testNoActiveSilences = amSilence.GetSilencesOK{
			Payload: testGettableSilences,
		}

		activeSilenceId      = "test-id"
		activeSilenceStatus  = amv2Models.SilenceStatusStateActive
		activeSilenceComment = "Silence for OSD worker node upgrade to version " + testVersion
		testActiveSilences   = amSilence.GetSilencesOK{
			Payload: []*amv2Models.GettableSilence{
				&amv2Models.GettableSilence{
					ID:        &activeSilenceId,
					Status:    &amv2Models.SilenceStatus{State:&activeSilenceStatus},
					Silence:   amv2Models.Silence{
						Comment:   &activeSilenceComment,
						CreatedBy: &testCreatedByOperator,
						EndsAt:    &testEnd,
						Matchers:  createDefaultMatchers(),
						StartsAt:  &testNow,
					},
				},
			},
		}
		ignoredControlPlaneCriticals = []string{"ignoredAlertSRE"}
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		silenceClient = NewMockAlertManagerSilencer(mockCtrl)
		mockKubeClient = mocks.NewMockClient(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	// Starting a Control Plane Silence
	Context("Creating a Control Plane silence", func() {
		It("Should not error on successfull maintenance start", func() {
			silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
			silenceClient.EXPECT().List(gomock.Any()).Return(&testNoActiveSilences, nil)
			end := time.Now().Add(90 * time.Minute)
			amm := alertManagerMaintenance{client: silenceClient}
			err := amm.StartControlPlane(end, testVersion, ignoredControlPlaneCriticals)
			Expect(err).Should(Not(HaveOccurred()))
		})
		It("Should error on failing to start maintenance", func() {
			silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error"))
			silenceClient.EXPECT().List(gomock.Any()).Return(&testNoActiveSilences, nil)
			end := time.Now().Add(90 * time.Minute)
			amm := alertManagerMaintenance{client: silenceClient}
			err := amm.StartControlPlane(end, testVersion, ignoredControlPlaneCriticals)
			Expect(err).Should(HaveOccurred())
		})
	})

	// Starting a worker silence
	Context("Creating a worker silence", func() {
		It("Should not error on successfull maintenance start", func() {
			silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			silenceClient.EXPECT().List(gomock.Any()).Return(&testNoActiveSilences, nil)
			end := time.Now().Add(90 * time.Minute)
			amm := alertManagerMaintenance{client: silenceClient}
			err := amm.SetWorker(end, testVersion)
			Expect(err).Should(Not(HaveOccurred()))
		})
		It("Should error on failing to start maintenance", func() {
			silenceClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error"))
			silenceClient.EXPECT().List(gomock.Any()).Return(&testNoActiveSilences, nil)
			end := time.Now().Add(90 * time.Minute)
			amm := alertManagerMaintenance{client: silenceClient}
			err := amm.SetWorker(end, testVersion)
			Expect(err).Should(HaveOccurred())
		})
	})

	// Updating an existing worker silence
	Context("Updating a worker silence", func() {
		It("Should update a silence if one already exists", func() {
			silenceClient.EXPECT().List(gomock.Any()).Return(&testActiveSilences, nil)
			silenceClient.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
			end := time.Now().Add(90 * time.Minute)
			amm := alertManagerMaintenance{client: silenceClient}
			err := amm.SetWorker(end, testVersion)
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

		It("Should find maintenances NOT created by the operator and not return an error", func() {
			testSilenceNotOwned := testSilence
			testSilenceNotOwned.CreatedBy = &testCreatedByTest

			// Create mock GettableSilence object to return
			gettableSilence := amv2Models.GettableSilence{
				ID:        &testId,
				Status:    &testStatus,
				UpdatedAt: &testUpdatedAt,
				Silence:   testSilenceNotOwned,
			}

			// Append GettableSilence to GettableSilences
			var gettableSilences amv2Models.GettableSilences
			gettableSilences = append(gettableSilences, &gettableSilence)

			activeSilences := amSilence.GetSilencesOK{
				Payload: gettableSilences,
			}

			silenceClient.EXPECT().List(gomock.Any()).Return(&activeSilences, nil)
			amm := alertManagerMaintenance{client: silenceClient}
			err := amm.EndSilences("")
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
			var gettableSilences amv2Models.GettableSilences
			gettableSilences = append(gettableSilences, &gettableSilence)

			activeSilences := amSilence.GetSilencesOK{
				Payload: gettableSilences,
			}

			silenceClient.EXPECT().List(gomock.Any()).Return(&activeSilences, nil)
			silenceClient.EXPECT().Delete(testId).Return(nil)
			amm := alertManagerMaintenance{client: silenceClient}
			err := amm.EndSilences("")
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
