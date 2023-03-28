package cmd

import (
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"time"

	"code.cloudfoundry.org/cflocal/fs"
	"code.cloudfoundry.org/cflocal/remote"
	"github.com/buildpack/forge/engine"
	forge "github.com/buildpack/forge/v2"
)

const (
	RunStack     = "packs/cflinuxfs3:run"
	BuildStack   = "packs/cflinuxfs3:build"
	NetworkStack = "packs/cflinuxfs3:network"
	//RunStack     = "cloudfoundry/cflinuxfs4"
	//BuildStack   = "cloudfoundry/cflinuxfs4"
	//NetworkStack = "cloudfoundry/cflinuxfs4"
)

type UI interface {
	Prompt(prompt string) string
	Output(format string, a ...interface{})
	Warn(format string, a ...interface{})
	Error(err error)
	Loading(message string, progress <-chan engine.Progress) error
}

//go:generate mockgen -package mocks -destination mocks/remote_app.go code.cloudfoundry.org/cflocal/cf/cmd RemoteApp
type RemoteApp interface {
	Command(name string) (string, error)
	Droplet(name string) (droplet io.ReadCloser, size int64, err error)
	SetDroplet(name string, droplet io.Reader, size int64) error
	Env(name string) (*remote.AppEnv, error)
	SetEnv(name string, env map[string]string) error
	Restart(name string) error
	Services(name string) (forge.Services, error)
	Forward(name string, services forge.Services) (forge.Services, *forge.ForwardDetails, error)
}

//go:generate mockgen -package mocks -destination mocks/local_app.go code.cloudfoundry.org/cflocal/cf/cmd LocalApp
type LocalApp interface {
	Tar(path string, excludes ...string) (io.ReadCloser, error)
}

//go:generate mockgen -package mocks -destination mocks/stager.go code.cloudfoundry.org/cflocal/cf/cmd Stager
type Stager interface {
	Stage(config *forge.StageConfig) (droplet engine.Stream, err error)
}

//go:generate mockgen -package mocks -destination mocks/runner.go code.cloudfoundry.org/cflocal/cf/cmd Runner
type Runner interface {
	Run(config *forge.RunConfig) (status int64, err error)
}

//go:generate mockgen -package mocks -destination mocks/exporter.go code.cloudfoundry.org/cflocal/cf/cmd Exporter
type Exporter interface {
	Export(config *forge.ExportConfig) (imageID string, err error)
}

//go:generate mockgen -package mocks -destination mocks/forwarder.go code.cloudfoundry.org/cflocal/cf/cmd Forwarder
type Forwarder interface {
	Forward(config *forge.ForwardConfig) (health <-chan string, done func(), id string, err error)
}

//go:generate mockgen -package mocks -destination mocks/image.go code.cloudfoundry.org/cflocal/cf/cmd Image
type Image interface {
	Pull(stack string) <-chan engine.Progress
}

//go:generate mockgen -package mocks -destination mocks/fs.go code.cloudfoundry.org/cflocal/cf/cmd FS
type FS interface {
	ReadFile(path string) (io.ReadCloser, int64, error)
	WriteFile(path string) (io.WriteCloser, error)
	OpenFile(path string) (fs.ReadResetWriteCloser, int64, error)
	Abs(path string) (string, error)
	Watch(dir string, wait time.Duration) (change <-chan time.Time, done chan<- struct{}, err error)
}

//go:generate mockgen -package mocks -destination mocks/help.go code.cloudfoundry.org/cflocal/cf/cmd Help
type Help interface {
	Short()
}

//go:generate mockgen -package mocks -destination mocks/config.go code.cloudfoundry.org/cflocal/cf/cmd Config
type Config interface {
	Load() (*forge.AppYAML, error)
	Save(localYML *forge.AppYAML) error
}

func parseOptions(args []string, f func(name string, set *flag.FlagSet)) error {
	if len(args) < 2 {
		return errors.New("app name required")
	}
	set := &flag.FlagSet{}
	set.SetOutput(ioutil.Discard)
	f(args[1], set)
	if err := set.Parse(args[2:]); err != nil {
		return err
	}
	if set.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	return nil
}

func getAppConfig(name string, localYML *forge.AppYAML) *forge.AppConfig {
	var app *forge.AppConfig
	for _, appConfig := range localYML.Applications {
		if appConfig.Name == name {
			app = appConfig
		}
	}
	if app == nil {
		app = &forge.AppConfig{Name: name}
		localYML.Applications = append(localYML.Applications, app)
	}
	return app
}

func getRemoteServices(app RemoteApp, serviceApp, forwardApp string) (forge.Services, *forge.ForwardDetails, error) {
	var services forge.Services

	if serviceApp == "" {
		serviceApp = forwardApp
	}
	if serviceApp != "" {
		var err error
		if services, err = app.Services(serviceApp); err != nil {
			return nil, nil, err
		}
	}
	if forwardApp != "" {
		return app.Forward(forwardApp, services)
	}
	return services, nil, nil
}
