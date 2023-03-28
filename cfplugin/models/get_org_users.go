package models

type GetOrgUsers_Model struct {
	Guid     string
	Username string
	IsAdmin  bool
	Roles    []string
}
