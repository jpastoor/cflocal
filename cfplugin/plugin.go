package cfplugin

import "code.cloudfoundry.org/cflocal/cfplugin/models"

/*
*

	Command interface needs to be implemented for a runnable plugin of `cf`

*
*/
type Plugin interface {
	Run(cliConnection CliConnection, args []string)
	GetMetadata() PluginMetadata
}

/*
*

	List of commands available to CliConnection variable passed into run

*
*/
type CliConnection interface {
	CliCommandWithoutTerminalOutput(args ...string) ([]string, error)
	CliCommand(args ...string) ([]string, error)
	GetCurrentOrg() (models.Organization, error)
	GetCurrentSpace() (models.Space, error)
	Username() (string, error)
	UserGuid() (string, error)
	UserEmail() (string, error)
	IsLoggedIn() (bool, error)
	// IsSSLDisabled returns true if and only if the user is connected to the Cloud Controller API with the
	// `--skip-ssl-validation` flag set unless the CLI configuration file cannot be read, in which case it
	// returns an error.
	IsSSLDisabled() (bool, error)
	HasOrganization() (bool, error)
	HasSpace() (bool, error)
	ApiEndpoint() (string, error)
	ApiVersion() (string, error)
	HasAPIEndpoint() (bool, error)
	LoggregatorEndpoint() (string, error)
	DopplerEndpoint() (string, error)
	AccessToken() (string, error)
	GetApp(string) (models.GetAppModel, error)
	GetApps() ([]models.GetAppsModel, error)
	GetOrgs() ([]models.GetOrgs_Model, error)
	GetSpaces() ([]models.GetSpaces_Model, error)
	GetOrgUsers(string, ...string) ([]models.GetOrgUsers_Model, error)
	GetSpaceUsers(string, string) ([]models.GetSpaceUsers_Model, error)
	GetServices() ([]models.GetServices_Model, error)
	GetService(string) (models.GetService_Model, error)
	GetOrg(string) (models.GetOrg_Model, error)
	GetSpace(string) (models.GetSpace_Model, error)
}

type VersionType struct {
	Major int
	Minor int
	Build int
}

type PluginMetadata struct {
	Name          string
	Version       VersionType
	MinCliVersion VersionType
	Commands      []Command
}

type Usage struct {
	Usage   string
	Options map[string]string
}

type Command struct {
	Name         string
	Alias        string
	HelpText     string
	UsageDetails Usage //Detail usage to be displayed in `cf help <cmd>`
}
