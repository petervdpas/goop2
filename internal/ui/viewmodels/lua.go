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
	CSRF       string
	Scripts    []LuaScript // chat scripts from lua/
	Functions  []LuaScript // data functions from lua/functions/
	Prefabs    []PrefabStatus
	ScriptDir  string // display path for the script directory

	// Editor state (when editing a script)
	EditName   string
	EditIsFunc bool   // true if editing a function (from functions/)
	Content    string
	Saved      bool
	Error      string

	// Whether Lua is currently enabled in config
	LuaEnabled bool
}
