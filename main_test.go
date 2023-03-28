package main_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/buildpack/forge/engine/docker/archive"
	gouuid "github.com/nu7hatch/gouuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/cflocal/fixtures"
)

var (
	pluginPath string
	tempHome   string
	spaceName  string

	resetEnv func()
)

var _ = BeforeSuite(func() {
	var err error

	tempHome, err = os.MkdirTemp("", "cflocal")
	Expect(err).NotTo(HaveOccurred())

	var set func(k, v string)
	set, resetEnv = setEnv()

	set("CF_HOME", tempHome)
	set("CF_PLUGIN_HOME", filepath.Join(tempHome, "plugins"))

	pluginPath, err = gexec.Build("code.cloudfoundry.org/cflocal", "-ldflags", "-X main.Version=100.200.300")
	Expect(err).NotTo(HaveOccurred())

	session, err := gexec.Start(exec.Command(pluginPath), GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, "1m").Should(gexec.Exit(0))
	Expect(session).To(gbytes.Say("Plugin successfully installed. Current version: 100.200.300"))

	spaceName = uniqueName("cflocal-space")

	cf("api", getEnv("CF_API"), ifSet("CF_SKIP_SSL_VALIDATION", "--skip-ssl-validation"))
	cf("auth", getEnv("CF_USER"), getEnv("CF_PASSWORD"))
	cf("create-space", spaceName, "-o", getEnv("CF_ORG"))
	cf("target", "-s", spaceName, "-o", getEnv("CF_ORG"))
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
	cf("delete-space", spaceName, "-o", getEnv("CF_ORG"), "-f")
	Expect(os.RemoveAll(tempHome)).To(Succeed())
	resetEnv()
})

var _ = Describe("CF Local", func() {
	Context("when executed directly", func() {
		It("should output a helpful usage message when run with help flags", func() {
			pluginCmd := exec.Command(pluginPath, "--help")
			session, err := gexec.Start(pluginCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session, "5s").Should(gexec.Exit(0))
			Expect(session).To(gbytes.Say("After installing, run: cf local help"))
		})

		It("should upgrade the plugin if it is already installed", func() {
			pluginCmd := exec.Command(pluginPath)
			session, err := gexec.Start(pluginCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session, "1m").Should(gexec.Exit(0))
			Expect(session).To(gbytes.Say("Plugin successfully upgraded. Current version: 100.200.300"))
		})

		It("should output an error message when the cf CLI is unavailable", func() {
			pluginCmd := exec.Command(pluginPath)
			pluginCmd.Env = []string{}
			session, err := gexec.Start(pluginCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session, "1m").Should(gexec.Exit(1))
			Expect(session.Out).To(gbytes.Say("Error: failed to determine cf CLI version"))
			Expect(session.Out).To(gbytes.Say("FAILED"))
		})
	})

	Context("when used as a cf CLI plugin", func() {
		var tempDir string

		BeforeEach(func() {
			wd, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			tempDir, err = os.MkdirTemp(wd, ".test.tmp")
			Expect(err).NotTo(HaveOccurred())
			Expect(archive.Copy("fixtures/go-app", tempDir)).To(Succeed())
			Expect(archive.Copy("fixtures/test-app", tempDir)).To(Succeed())
			Expect(archive.Copy("fixtures/staged-app", tempDir)).To(Succeed())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tempDir)).To(Succeed())
		})

		FIt("should setup the staging and running environments to mimic CF", func() {
			By("staging", func() {
				stageCmd := exec.Command("cf", "local", "stage", "some-name")
				stageCmd.Dir = filepath.Join(tempDir, "test-app")
				session, err := gexec.Start(stageCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))
				Expect(session).To(gbytes.Say("Compile message from stdout."))
				Expect(session).To(gbytes.Say("Cache not detected."))
				Expect(session.Out.Contents()).To(ContainSubstring("Compile message from stderr."))

				Expect(os.Stat(filepath.Join(tempDir, "test-app", "some-name.droplet"))).To(WithTransform(os.FileInfo.Size, BeNumerically(">", 0)))
				Expect(os.Stat(filepath.Join(tempDir, "test-app", ".some-name.cache"))).To(WithTransform(os.FileInfo.Size, BeNumerically(">", 0)))
			})

			By("restaging", func() {
				stageCmd := exec.Command("cf", "local", "stage", "some-name")
				stageCmd.Dir = filepath.Join(tempDir, "test-app")
				session, err := gexec.Start(stageCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))
				Expect(session).To(gbytes.Say("Compile message from stdout."))
				Expect(session).To(gbytes.Say("Cache detected."))
				Expect(session.Out.Contents()).To(ContainSubstring("Compile message from stderr."))
			})

			By("running", func() {
				runCmd := exec.Command("cf", "local", "run", "some-name")
				runCmd.Dir = filepath.Join(tempDir, "test-app")
				setpgid(runCmd)
				session, err := gexec.Start(runCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				message := `Running some-name on port ([\d]+)\.\.\.`
				Eventually(session, "10s").Should(gbytes.Say(message))
				port := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
				url := fmt.Sprintf("http://localhost:%s", port)

				response := fmt.Sprintf(
					"Staging:\n%s\nRunning:\n%s\n",
					strings.Join(fixtures.StagingEnv(
						"some-name",
						512,
						"some-type",
						"LC_COLLATE=C",
						"TEST_ENV_KEY=test-env-value",
						"TEST_STAGING_ENV_KEY=test-staging-env-value",
					), "\n"),
					strings.Join(fixtures.RunningEnv("some-name",
						512,
						"some-type",
						"LC_COLLATE=C",
						"TEST_ENV_KEY=test-env-value",
						"TEST_RUNNING_ENV_KEY=test-running-env-value",
					), "\n"),
				)
				Expect(get(url, "10s")).To(MatchRegexp("(?m)" + response))

				Eventually(session).Should(gbytes.Say("Log message from stdout."))
				Eventually(session.Out.Contents).Should(ContainSubstring("Log message from stderr."))

				kill(runCmd)
				Eventually(session, "5s").Should(gexec.Exit(130))
			})

			By("running wih a shell", func() {
				runCmd := exec.Command("cf", "local", "run", "some-name", "-t")
				runCmd.Dir = filepath.Join(tempDir, "test-app")
				in, err := runCmd.StdinPipe()
				Expect(err).NotTo(HaveOccurred())
				session, err := gexec.Start(runCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10s").Should(gbytes.Say(`vcap@some-name:~\$ `))
				_, err = fmt.Fprintln(in, "env|sort")
				Expect(err).NotTo(HaveOccurred())
				Eventually(session, "10s").Should(gbytes.Say(`env[|]sort\r\n`))

				Eventually(func() string {
					return strings.Replace(string(session.Out.Contents()), "\r", "", -1)
				}, "5s").Should(MatchRegexp(
					"(?m)" + strings.Join(fixtures.RunningEnv("some-name",
						512,
						"some-type",
						"LC_COLLATE=C",
						"SHLVL=2",
						"TERM=xterm",
						"TEST_ENV_KEY=test-env-value",
						"TEST_RUNNING_ENV_KEY=test-running-env-value",
					), "\n"),
				))
				_, err = fmt.Fprintln(in, "exit")
				Expect(err).NotTo(HaveOccurred())
				Eventually(session, "10s").Should(gexec.Exit(0))
			})
		})

		It("should successfully stage and run apps with a variety of buildpacks", func() {
			staticfileResp, err := http.Get("https://github.com/cloudfoundry/staticfile-buildpack/releases/download/v1.4.16/staticfile-buildpack-v1.4.16.zip")
			Expect(err).NotTo(HaveOccurred())
			defer staticfileResp.Body.Close()
			staticfile, err := ioutil.TempFile("", "cflocal-test")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(staticfile.Name())
			defer staticfile.Close()
			_, err = io.Copy(staticfile, staticfileResp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(staticfile.Sync()).To(Succeed())

			Expect(archive.Copy("fixtures/multi-shims/.", filepath.Join(tempDir, "go-app"))).To(Succeed())

			By("staging", func() {
				stageCmd := exec.Command(
					"cf", "local", "stage", "some-name",
					"-b", "https://github.com/cloudfoundry/ruby-buildpack/releases/download/v1.7.3/ruby-buildpack-v1.7.3.zip",
					"-b", "https://github.com/cloudfoundry/python-buildpack#v1.5.26",
					"-b", staticfile.Name(), "-b", staticfile.Name(),
					"-b", "go_buildpack",
				)
				stageCmd.Dir = filepath.Join(tempDir, "go-app")
				session, err := gexec.Start(stageCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))
				Expect(session).To(gbytes.Say("Successfully staged: some-name"))
			})

			By("running", func() {
				runCmd := exec.Command("cf", "local", "run", "some-name")
				runCmd.Dir = filepath.Join(tempDir, "go-app")
				setpgid(runCmd)
				session, err := gexec.Start(runCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				message := `Running some-name on port ([\d]+)\.\.\.`
				Eventually(session, "10s").Should(gbytes.Say(message))
				port := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
				versions := strings.Join([]string{
					"ruby=--version",
					"python=--version",
					"nginx=-v",
				}, "&")
				url := fmt.Sprintf("http://localhost:%s/run?%s", port, versions)

				response := get(url, "10s")
				Expect(response).To(ContainSubstring("ruby 2.4.2p198 (2017-09-14 revision 59899) [x86_64-linux]"))
				Expect(response).To(ContainSubstring("Python 2.7.14"))
				Expect(response).To(ContainSubstring("nginx version: nginx/1.13.5"))

				kill(runCmd)
				Eventually(session, "5s").Should(gexec.Exit(130))
			})
		})

		It("should successfully stage, run, and push a local app", func() {
			By("staging", func() {
				stageCmd := exec.Command("cf", "local", "stage", "some-name")
				stageCmd.Dir = filepath.Join(tempDir, "go-app")
				session, err := gexec.Start(stageCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))
				Expect(session).To(gbytes.Say("Successfully staged: some-name"))
			})

			By("running", func() {
				runCmd := exec.Command("cf", "local", "run", "some-name")
				runCmd.Dir = filepath.Join(tempDir, "go-app")
				setpgid(runCmd)
				session, err := gexec.Start(runCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				message := `Running some-name on port ([\d]+)\.\.\.`
				Eventually(session, "10s").Should(gbytes.Say(message))
				port := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
				url := fmt.Sprintf("http://localhost:%s/some-path", port)

				Expect(get(url, "10s")).To(Equal("Path: /some-path"))
				kill(runCmd)

				Eventually(session, "5s").Should(gexec.Exit(130))
			})

			By("pushing", func() {
				cfPushCmd := exec.Command("cf", "push", "some-name", "--no-start", "--random-route")
				cfPushCmd.Dir = filepath.Join(tempDir, "test-app")
				cfSession, err := gexec.Start(cfPushCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(cfSession, "4m").Should(gexec.Exit(0))

				pushCmd := exec.Command("cf", "local", "push", "some-name")
				pushCmd.Dir = filepath.Join(tempDir, "go-app")
				session, err := gexec.Start(pushCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))

				message := `\nurls: (\S+)\n`
				Expect(session).To(gbytes.Say(message))
				route := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
				url := fmt.Sprintf("http://%s/some-path", route)
				Expect(get(url, "10s")).To(Equal("Path: /some-path"))
			})
		})

		It("should successfully pull, run, and push an app from CF", func() {
			Expect(os.Mkdir(filepath.Join(tempDir, "local-app"), 0777)).To(Succeed())

			By("pulling", func() {
				cfPushCmd := exec.Command("cf", "push", "some-name", "--random-route")
				cfPushCmd.Dir = filepath.Join(tempDir, "go-app")
				cfSession, err := gexec.Start(cfPushCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(cfSession, "5m").Should(gexec.Exit(0))

				pullCmd := exec.Command("cf", "local", "pull", "some-name")
				pullCmd.Dir = filepath.Join(tempDir, "local-app")
				session, err := gexec.Start(pullCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))
				Expect(session).To(gbytes.Say("Successfully downloaded: some-name"))

				cf("delete", "some-name", "-f")
			})

			By("running", func() {
				runCmd := exec.Command("cf", "local", "run", "some-name")
				runCmd.Dir = filepath.Join(tempDir, "local-app")
				setpgid(runCmd)
				session, err := gexec.Start(runCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				message := `Running some-name on port ([\d]+)\.\.\.`
				Eventually(session, "10s").Should(gbytes.Say(message))
				port := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
				url := fmt.Sprintf("http://localhost:%s/some-path", port)

				Expect(get(url, "10s")).To(Equal("Path: /some-path"))
				kill(runCmd)

				Eventually(session, "5s").Should(gexec.Exit(130))
			})

			By("pushing", func() {
				cfPushCmd := exec.Command("cf", "push", "some-name", "--no-start", "--random-route")
				cfPushCmd.Dir = filepath.Join(tempDir, "test-app")
				cfSession, err := gexec.Start(cfPushCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(cfSession, "4m").Should(gexec.Exit(0))

				pushCmd := exec.Command("cf", "local", "push", "some-name")
				pushCmd.Dir = filepath.Join(tempDir, "local-app")
				session, err := gexec.Start(pushCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))

				message := `\nurls: (\S+)\n`
				Expect(session).To(gbytes.Say(message))
				route := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
				url := fmt.Sprintf("http://%s/some-path", route)
				Expect(get(url, "10s")).To(Equal("Path: /some-path"))
			})
		})

		It("should forward service connections via ssh tunnels", func() {
			By("pushing", func() {
				cfPushCmd := exec.Command("cf", "push", "remote-app", "--no-start", "--random-route")
				cfPushCmd.Dir = filepath.Join(tempDir, "go-app")
				cfSession, err := gexec.Start(cfPushCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cfSession, "4m").Should(gexec.Exit(0))
			})

			By("creating", func() {
				creds := `{"uri": "http://example.com:80", "host_header": "example.com"}`
				cfCUPSCmd := exec.Command("cf", "create-user-provided-service", "some-service", "-p", creds)
				cfSession, err := gexec.Start(cfCUPSCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cfSession, "10s").Should(gexec.Exit(0))
			})

			By("binding", func() {
				cfBSCmd := exec.Command("cf", "bind-service", "remote-app", "some-service")
				cfSession, err := gexec.Start(cfBSCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cfSession, "10s").Should(gexec.Exit(0))
			})

			By("starting", func() {
				cfStart := exec.Command("cf", "start", "remote-app")
				cfSession, err := gexec.Start(cfStart, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())
				Eventually(cfSession, "4m").Should(gexec.Exit(0))
			})

			By("staging", func() {
				stageCmd := exec.Command("cf", "local", "stage", "some-name", "-f", "remote-app")
				stageCmd.Dir = filepath.Join(tempDir, "go-app")
				session, err := gexec.Start(stageCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10m").Should(gexec.Exit(0))
				Expect(session).To(gbytes.Say("Successfully staged: some-name"))
			})

			By("running", func() {
				runCmd := exec.Command("cf", "local", "run", "some-name", "-f", "remote-app")
				runCmd.Dir = filepath.Join(tempDir, "go-app")
				setpgid(runCmd)
				session, err := gexec.Start(runCmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, "10s").Should(gbytes.Say(`Forwarding: some-service:user-provided\[0\]`))
				message := `Running some-name on port ([\d]+)\.\.\.`
				Eventually(session, "10s").Should(gbytes.Say(message))
				port := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
				url := fmt.Sprintf("http://localhost:%s/services", port)

				response := get(url, "10s")
				Expect(response).To(ContainSubstring("Name: some-service"))
				Expect(response).To(ContainSubstring("URI: http://localhost:40000"))
				Expect(response).To(ContainSubstring("Example Domain"))

				kill(runCmd)
				Eventually(session, "5s").Should(gexec.Exit(130))
			})
		})

		// TODO: test modified VCAP_SERVICES (without tunnels) during staging

		if os.Getenv("SKIP_VOLUME_SPECS") != "true" {
			It("should use a volume to stage and run an app", func() {
				By("staging", func() {
					stageCmd := exec.Command("cf", "local", "stage", "some-name")
					stageCmd.Dir = filepath.Join(tempDir, "go-app")
					session, err := gexec.Start(stageCmd, GinkgoWriter, GinkgoWriter)
					Expect(err).NotTo(HaveOccurred())

					Eventually(session, "10m").Should(gexec.Exit(0))
					Expect(session).To(gbytes.Say("Successfully staged: some-name"))
				})

				By("running", func() {
					runCmd := exec.Command("cf", "local", "run", "some-name", "-d", filepath.Join(tempDir, "staged-app"), "-w")
					runCmd.Dir = filepath.Join(tempDir, "go-app")
					setpgid(runCmd)
					session, err := gexec.Start(runCmd, GinkgoWriter, GinkgoWriter)
					Expect(err).NotTo(HaveOccurred())

					message := `Running some-name on port ([\d]+)\.\.\.`
					Eventually(session, "10s").Should(gbytes.Say(message))
					port := regexp.MustCompile(message).FindSubmatch(session.Out.Contents())[1]
					url := fmt.Sprintf("http://localhost:%s", port)

					Expect(get(url, "10s")).To(Equal("some-contents\n"))
					Expect(ioutil.WriteFile(filepath.Join(tempDir, "staged-app", "file"), []byte("some-other-contents\n"), 0666)).To(Succeed())
					Eventually(func() string { return get(url, "10s") }, "10s").Should(Equal("some-other-contents\n"))

					kill(runCmd)
					Eventually(session, "5s").Should(gexec.Exit(130))
				})
			})
		}
	})
})

func setEnv() (set func(k, v string), reset func()) {
	var new []string
	saved := map[string]string{}
	return func(k, v string) {
			if old, ok := os.LookupEnv(k); ok {
				saved[k] = old
			} else {
				new = append(new, k)
			}
			if err := os.Setenv(k, v); err != nil {
				Fail(err.Error(), 1)
			}
		}, func() {
			for k, v := range saved {
				if err := os.Setenv(k, v); err != nil {
					Fail(err.Error(), 1)
				}
				delete(saved, k)
			}
			for _, k := range new {
				if err := os.Unsetenv(k); err != nil {
					Fail(err.Error(), 1)
				}
			}
			new = nil
		}
}

func getEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		Fail("Not set: " + k)
	}
	return v
}

func ifSet(k string, ret string) string {
	if v := os.Getenv(k); v != "true" && v != "1" {
		return ""
	}
	return ret
}

func uniqueName(s string) string {
	guid, err := gouuid.NewV4()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return fmt.Sprintf("%s-%s", s, guid)
}

func cf(args ...string) {

	nonEmptyArgs := make([]string, 0)
	for _, arg := range args {
		if arg != "" {
			nonEmptyArgs = append(nonEmptyArgs, arg)
		}
	}

	cmd := exec.Command("cf", nonEmptyArgs...)
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, session, "10s").Should(gexec.Exit(0))
}

func get(url, timeout string) string {
	var body io.ReadCloser
	EventuallyWithOffset(1, func() error {
		response, err := http.Get(url)
		if err != nil {
			return err
		}
		body = response.Body
		return nil
	}, timeout).Should(Succeed())
	defer body.Close()
	bodyBytes, err := ioutil.ReadAll(body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(bodyBytes)
}
