package models

type GetSpaceUsers_Model struct {
	Guid     string
	Username string
	IsAdmin  bool
	Roles    []string
}
