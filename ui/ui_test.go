package ui_test

import (
	"errors"
	"io"

	"code.cloudfoundry.org/cflocal/ui"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/buildpack/forge/engine"
)

var _ = Describe("UI", func() {
	var (
		out, err, in *gbytes.Buffer
		myUi         *ui.UI
	)

	BeforeEach(func() {
		out = gbytes.NewBuffer()
		err = gbytes.NewBuffer()
		in = gbytes.NewBuffer()
		myUi = &ui.UI{Out: out, Err: err, In: in}
	})

	Describe("#Prompt", func() {
		It("should output the prompt and return the user's entry", func() {
			io.WriteString(in, "some answer\n")
			response := myUi.Prompt("some question")
			Expect(out).To(gbytes.Say("some question"))
			Expect(response).To(Equal("some answer"))
		})

		Context("when the input cannot be read", func() {
			It("should output the prompt and return an empty string", func() {
				response := myUi.Prompt("some question")
				Expect(out).To(gbytes.Say("some question"))
				Expect(response).To(BeEmpty())
			})
		})
	})

	Describe("#Output", func() {
		It("should output the provided format string", func() {
			myUi.Output("%s format", "some")
			Expect(out).To(gbytes.Say("some format"))
		})
	})

	Describe("#Warning", func() {
		Context("when stderr is connected", func() {
			It("should output the provided warning as to stderr", func() {
				myUi.ErrIsTerm = true
				myUi.Warn("%s warning", "some")
				Expect(err).To(gbytes.Say("Warning: some warning"))
			})
		})

		Context("when stderr is not connected", func() {
			It("should output the provided warning to stdout", func() {
				myUi.Warn("%s warning", "some")
				Expect(out).To(gbytes.Say("Warning: some warning"))
			})
		})
	})

	Describe("#Error", func() {
		Context("when stderr is connected", func() {
			It("should output the provided error as to stderr followed by FAILED", func() {
				myUi.ErrIsTerm = true
				myUi.Error(errors.New("some error"))
				Expect(err).To(gbytes.Say("Error: some error"))
				Expect(out).To(gbytes.Say("FAILED"))
			})
		})

		Context("when stderr is not connected", func() {
			It("should output the provided error to stdout followed by FAILED", func() {
				myUi.Error(errors.New("some error"))
				Expect(out).To(gbytes.Say("Error: some error"))
				Expect(out).To(gbytes.Say("FAILED"))
			})
		})
	})

	Describe("#Loading", func() {
		It("should drain the provided channel", func() {
			progress := make(chan engine.Progress, 2)
			progress <- mockProgress{}
			progress <- mockProgress{}
			close(progress)
			Expect(myUi.Loading("some-message", progress)).To(Succeed())
			Expect(progress).To(BeClosed())
		})

		It("should return the last error sent", func() {
			progress := make(chan engine.Progress, 3)
			progress <- mockProgress{err: errors.New("first error")}
			progress <- mockProgress{err: errors.New("second error")}
			progress <- mockProgress{}
			close(progress)
			err := myUi.Loading("some-message", progress)
			Expect(err).To(MatchError("second error"))
			Expect(progress).To(BeClosed())
		})

		// TODO: test loading bar
	})
})

type mockProgress struct {
	err error
}

func (m mockProgress) Status() (string, error) {
	return "some-progress", m.err
}
