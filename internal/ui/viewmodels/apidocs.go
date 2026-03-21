package viewmodels

import "html/template"

type ApiDocsVM struct {
	BaseVM
	SDKDoc template.HTML
	LuaDoc template.HTML
}
