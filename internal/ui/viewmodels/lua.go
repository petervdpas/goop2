// internal/ui/viewmodels/lua.go

package viewmodels

type LuaScript struct {
	Name string // filename without .lua
	Size int64
}

type PrefabScriptStatus struct {
	Name      string // script name without .lua
	Installed bool
}

type PrefabStatus struct {
	Name        string
	Description string
	Icon        string
	Dir         string
	Scripts     []PrefabScriptStatus
	AllInstalled bool
}

type LuaVM struct {
	BaseVM
	CSRF    string
	Scripts []LuaScript
	Prefabs []PrefabStatus

	// Editor state (when editing a script)
	EditName string
	Content  string
	Saved    bool
	Error    string

	// Whether Lua is currently enabled in config
	LuaEnabled bool
}
