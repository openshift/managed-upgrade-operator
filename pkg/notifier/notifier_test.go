package notifier

// import (
// 	. "github.com/onsi/ginkgo"
// 	. "github.com/onsi/gomega"

// 	"github.com/openshift/managed-upgrade-operator/pkg/notifier/mocks"
// )

// var _ = Describe("Notifier", func() {
// 	var (
// 		mockClient                      *mocks.MockClient
// 		mockConfigManagerBuilder        *mocks.MockConfigManagerBuilder
// 		mockUpgradeConfigManagerBuilder *mocks.MockUpgradeConfigManagerBuilder
// 		mockNotifier                    *mocks.MockNotifier
// 		notifierBuilder                 NotifierBuilder
// 	)

// 	BeforeEach(func() {
// 		mockClient = &mocks.MockClient{}
// 		mockConfigManagerBuilder = &mocks.MockConfigManagerBuilder{}
// 		mockUpgradeConfigManagerBuilder = &mocks.MockUpgradeConfigManagerBuilder{}
// 		mockNotifier = &mocks.MockNotifier{}

// 		notifierBuilder = NewBuilder()
// 	})

// 	Describe("NewBuilder", func() {
// 		It("should create a new NotifierBuilder instance", func() {
// 			builder := NewBuilder()
// 			Expect(builder).ToNot(BeNil())
// 		})
// 	})

// 	Describe("New", func() {
// 		It("should create a new Notifier instance", func() {
// 			notifier, err := notifierBuilder.New(mockClient, mockConfigManagerBuilder, mockUpgradeConfigManagerBuilder)
// 			Expect(err).ToNot(HaveOccurred())
// 			Expect(notifier).ToNot(BeNil())
// 		})

// 		It("should return an error if no valid notifier is configured", func() {
// 			mockConfigManagerBuilder.EXPECT().Build().Return(nil, ErrNoNotifierConfigured)

// 			_, err := notifierBuilder.New(mockClient, mockConfigManagerBuilder, mockUpgradeConfigManagerBuilder)
// 			Expect(err).To(HaveOccurred())
// 			Expect(err).To(MatchError(ErrNoNotifierConfigured))
// 		})
// 	})

// 	Describe("NotifyState", func() {
// 		It("should notify the state with the given value and description", func() {
// 			mockEXPECT().NotifyState(MuoStateCompleted, "Upgrade completed successfully").Return(nil)

// 			notifier, _ := notifierBuilder.New(mockClient, mockConfigManagerBuilder, mockUpgradeConfigManagerBuilder)
// 			err := NotifyState(MuoStateCompleted, "Upgrade completed successfully")
// 			Expect(err).ToNot(HaveOccurred())
// 		})
// 	})
// })
