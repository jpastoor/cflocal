package cmd_test

import (
	"io/ioutil"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	. "code.cloudfoundry.org/cflocal/cf/cmd"
	"code.cloudfoundry.org/cflocal/cf/cmd/mocks"
	sharedmocks "code.cloudfoundry.org/cflocal/mocks"
	"github.com/buildpack/forge/engine"
	forge "github.com/buildpack/forge/v2"
)

var _ = Describe("Export", func() {
	var (
		mockCtrl     *gomock.Controller
		mockUI       *sharedmocks.MockUI
		mockExporter *mocks.MockExporter
		mockImage    *mocks.MockImage
		mockFS       *mocks.MockFS
		mockHelp     *mocks.MockHelp
		mockConfig   *mocks.MockConfig
		cmd          *Export
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockUI = sharedmocks.NewMockUI()
		mockExporter = mocks.NewMockExporter(mockCtrl)
		mockImage = mocks.NewMockImage(mockCtrl)
		mockFS = mocks.NewMockFS(mockCtrl)
		mockHelp = mocks.NewMockHelp(mockCtrl)
		mockConfig = mocks.NewMockConfig(mockCtrl)
		cmd = &Export{
			UI:       mockUI,
			Exporter: mockExporter,
			Image:    mockImage,
			FS:       mockFS,
			Help:     mockHelp,
			Config:   mockConfig,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("#Match", func() {
		It("should return true when the first argument is export", func() {
			Expect(cmd.Match([]string{"export"})).To(BeTrue())
			Expect(cmd.Match([]string{"not-export"})).To(BeFalse())
			Expect(cmd.Match([]string{})).To(BeFalse())
			Expect(cmd.Match(nil)).To(BeFalse())
		})
	})

	Describe("#Run", func() {
		It("should export a droplet as a Docker image", func() {
			progress := make(chan engine.Progress, 1)
			progress <- mockProgress{Value: "some-progress"}
			close(progress)
			droplet := sharedmocks.NewMockBuffer("some-droplet")
			localYML := &forge.AppYAML{
				Applications: []*forge.AppConfig{
					{
						Name: "some-other-app",
					},
					{
						Name:     "some-app",
						Env:      map[string]string{"a": "b"},
						Services: forge.Services{"some": {{Name: "services"}}},
					},
				},
			}
			mockConfig.EXPECT().Load().Return(localYML, nil)
			mockFS.EXPECT().ReadFile("./some-app.droplet").Return(droplet, int64(100), nil)
			gomock.InOrder(
				mockImage.EXPECT().Pull(RunStack).Return(progress),
				mockExporter.EXPECT().Export(gomock.Any()).Do(
					func(config *forge.ExportConfig) {
						Expect(ioutil.ReadAll(config.Droplet)).To(Equal([]byte("some-droplet")))
						Expect(config.Stack).To(Equal(RunStack))
						Expect(config.Ref).To(Equal("some-reference"))
						Expect(config.OutputDir).To(Equal("/home/vcap"))
						Expect(config.WorkingDir).To(Equal("/home/vcap/app"))
						Expect(config.AppConfig).To(Equal(&forge.AppConfig{
							Name:     "some-app",
							Env:      map[string]string{"a": "b"},
							Services: forge.Services{"some": {{Name: "services"}}},
						}))
					},
				).Return("some-id", nil),
			)

			Expect(cmd.Run([]string{"export", "some-app", "-r", "some-reference"})).To(Succeed())
			Expect(droplet.Result()).To(BeEmpty())
			Expect(mockUI.Out).To(gbytes.Say("Exported some-app as some-reference with ID: some-id"))
			Expect(mockUI.Progress).To(Receive(Equal(mockProgress{Value: "some-progress"})))
		})

		// TODO: test without reference
	})
})
