package rendezvous

// StoreMeta holds metadata for a template (built-in, store, or local).
type StoreMeta struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Category     string                 `json:"category"`
	Icon         string                 `json:"icon"`
	Dir          string                 `json:"dir"`
	Source       string                 `json:"source"`
	Tables       map[string]TablePolicy `json:"tables,omitempty"`  // legacy
	Schemas      []string               `json:"schemas,omitempty"` // ORM table names owned by this template
	RequireEmail bool                   `json:"require_email,omitempty"`
	DefaultRole  string                 `json:"default_role,omitempty"`
}

// TablePolicy holds per-table configuration from a template manifest (legacy).
type TablePolicy struct {
	InsertPolicy string `json:"insert_policy"`
}
