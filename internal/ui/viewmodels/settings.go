
package viewmodels

import "github.com/petervdpas/goop2/internal/config"

type SettingsVM struct {
	BaseVM
	CfgPath    string
	CSRF       string
	AvatarHash string

	Saved bool
	Error string

	Cfg config.Config
}
