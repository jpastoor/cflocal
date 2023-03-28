package cmd_test

import (
	"code.cloudfoundry.org/cflocal/cf/cmd"
	forge "github.com/buildpack/forge/v2"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"code.cloudfoundry.org/cflocal/cf/cmd/mocks"
	sharedmocks "code.cloudfoundry.org/cflocal/mocks"
	"code.cloudfoundry.org/cflocal/remote"
)

var _ = Describe("Pull", func() {
	var (
		mockCtrl      *gomock.Controller
		mockUI        *sharedmocks.MockUI
		mockRemoteApp *mocks.MockRemoteApp
		mockFS        *mocks.MockFS
		mockHelp      *mocks.MockHelp
		mockConfig    *mocks.MockConfig
		cmdPull       *cmd.Pull
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockUI = sharedmocks.NewMockUI()
		mockRemoteApp = mocks.NewMockRemoteApp(mockCtrl)
		mockFS = mocks.NewMockFS(mockCtrl)
		mockHelp = mocks.NewMockHelp(mockCtrl)
		mockConfig = mocks.NewMockConfig(mockCtrl)
		cmdPull = &cmd.Pull{
			UI:        mockUI,
			RemoteApp: mockRemoteApp,
			FS:        mockFS,
			Help:      mockHelp,
			Config:    mockConfig,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("#Match", func() {
		It("should return true when the first argument is pull", func() {
			Expect(cmdPull.Match([]string{"pull"})).To(BeTrue())
			Expect(cmdPull.Match([]string{"not-pull"})).To(BeFalse())
			Expect(cmdPull.Match([]string{})).To(BeFalse())
			Expect(cmdPull.Match(nil)).To(BeFalse())
		})
	})

	Describe("#Run", func() {
		It("should download a droplet and save its env vars", func() {
			droplet := sharedmocks.NewMockBuffer("some-droplet")
			file := sharedmocks.NewMockBuffer("")
			env := &remote.AppEnv{
				Staging: map[string]string{"a": "b"},
				Running: map[string]string{"c": "d"},
				App:     map[string]string{"e": "f"},
			}
			oldLocalYML := &forge.AppYAML{
				Applications: []*forge.AppConfig{
					{Name: "some-other-app"},
					{
						Name:       "some-app",
						Command:    "some-old-command",
						StagingEnv: map[string]string{"g": "h"},
						RunningEnv: map[string]string{"i": "j"},
						Env:        map[string]string{"k": "l"},
					},
				},
			}
			newLocalYML := &forge.AppYAML{
				Applications: []*forge.AppConfig{
					{Name: "some-other-app"},
					{
						Name:       "some-app",
						Command:    "some-command",
						StagingEnv: map[string]string{"a": "b"},
						RunningEnv: map[string]string{"c": "d"},
						Env:        map[string]string{"e": "f"},
					},
				},
			}
			mockRemoteApp.EXPECT().Droplet("some-app").Return(droplet, int64(100), nil)
			mockFS.EXPECT().WriteFile("./some-app.droplet").Return(file, nil)
			mockConfig.EXPECT().Load().Return(oldLocalYML, nil)
			mockRemoteApp.EXPECT().Env("some-app").Return(env, nil)
			mockRemoteApp.EXPECT().Command("some-app").Return("some-command", nil)
			mockConfig.EXPECT().Save(newLocalYML)

			Expect(cmdPull.Run([]string{"pull", "some-app"})).To(Succeed())
			Expect(file.Result()).To(Equal("some-droplet"))
			Expect(droplet.Result()).To(BeEmpty())
			Expect(mockUI.Out).To(gbytes.Say("Successfully downloaded: some-app"))
		})

		// TODO: test when app isn't in local.yml
	})
})
