package template

import (
	"crypto/rand"
	"encoding/hex"
	"log"

	"github.com/petervdpas/goop2/internal/storage"
)

// ApplyConfig holds the parameters for Apply.
type ApplyConfig struct {
	DB            *storage.DB
	TemplateName  string
	DefaultRole   string
	SchemaInfo    SchemaInfo
}

// Apply ensures the template group exists (or is reused/closed) based on
// the schema analysis. It sets default_role and available roles on the group,
// and persists the group ID in _meta.
//
// Returns the group ID (empty if no group was needed).
func (h *Handler) Apply(cfg ApplyConfig) string {
	if h.grpMgr == nil {
		return ""
	}

	existingGroupID := ""
	existingGroupTemplate := ""
	if cfg.DB != nil {
		existingGroupID = cfg.DB.GetMeta("template_group_id")
		existingGroupTemplate = cfg.DB.GetMeta("template_group_name")
	}

	sameTemplate := existingGroupID != "" && existingGroupTemplate == cfg.TemplateName
	if cfg.SchemaInfo.NeedsGroup && sameTemplate {
		log.Printf("TEMPLATE: reusing existing group %s", existingGroupID)
		h.configureGroup(existingGroupID, cfg.DefaultRole, cfg.SchemaInfo.Roles)
		return existingGroupID
	}

	// Switching templates — close ALL groups owned by the old template
	if existingGroupTemplate != "" {
		h.closeTemplateGroups(existingGroupTemplate)
	}
	if cfg.DB != nil {
		cfg.DB.SetMeta("template_group_id", "")
		cfg.DB.SetMeta("template_group_name", "")
	}

	if !cfg.SchemaInfo.NeedsGroup {
		return ""
	}

	// Create new template group
	groupName := cfg.TemplateName + " Co-authors"
	if cfg.TemplateName == "" {
		groupName = "Co-authors"
	}
	newID := generateGroupID()
	if err := h.grpMgr.CreateGroup(newID, groupName, GroupTypeName, cfg.TemplateName, 0, false); err != nil {
		log.Printf("TEMPLATE: failed to create group: %v", err)
		return ""
	}
	log.Printf("TEMPLATE: created group %q (%s)", groupName, newID)

	if err := h.grpMgr.JoinOwnGroup(newID); err != nil {
		log.Printf("TEMPLATE: failed to auto-join group: %v", err)
	}

	h.configureGroup(newID, cfg.DefaultRole, cfg.SchemaInfo.Roles)

	if cfg.DB != nil {
		cfg.DB.SetMeta("template_group_id", newID)
		cfg.DB.SetMeta("template_group_name", cfg.TemplateName)
	}

	return newID
}

// closeTemplateGroups closes all groups whose GroupContext matches the template name.
func (h *Handler) closeTemplateGroups(templateName string) {
	groups, err := h.grpMgr.ListHostedGroups()
	if err != nil {
		return
	}
	for _, g := range groups {
		if g.GroupType == GroupTypeName && g.GroupContext == templateName {
			_ = h.grpMgr.CloseGroup(g.ID)
			log.Printf("TEMPLATE: closed group %s (context=%s)", g.ID, g.GroupContext)
		}
	}
	for _, c := range h.cleaners {
		c.CloseByContext(templateName)
	}
}

func (h *Handler) configureGroup(groupID, defaultRole string, roles []string) {
	if defaultRole != "" {
		if err := h.grpMgr.SetDefaultRole(groupID, defaultRole); err != nil {
			log.Printf("TEMPLATE: failed to set default role: %v", err)
		}
	}
	if len(roles) > 0 {
		if err := h.grpMgr.SetGroupRoles(groupID, roles); err != nil {
			log.Printf("TEMPLATE: failed to set group roles: %v", err)
		}
	}
}

func generateGroupID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
