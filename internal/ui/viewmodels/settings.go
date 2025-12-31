// internal/ui/viewmodels/settings.go

package viewmodels

import "goop/internal/config"

type SettingsVM struct {
	BaseVM
	CfgPath string
	CSRF    string

	Saved bool
	Error string

	Cfg config.Config
}
