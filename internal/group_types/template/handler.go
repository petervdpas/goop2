package template

import (
	"log"

	"github.com/petervdpas/goop2/internal/group"
)

const GroupTypeName = "template"

// Handler implements group.TypeHandler for template groups.
type Handler struct {
	grpMgr *group.Manager
}

// New creates a template handler and registers it with the group manager.
func New(grpMgr *group.Manager) *Handler {
	h := &Handler{grpMgr: grpMgr}
	grpMgr.RegisterType(GroupTypeName, h)
	return h
}

func (h *Handler) Flags() group.TypeFlags {
	return group.TypeFlags{HostCanJoin: true}
}

func (h *Handler) OnCreate(_, _ string, _ int, _ bool) error { return nil }

func (h *Handler) OnJoin(groupID, peerID string, isHost bool) {
	if !isHost {
		log.Printf("TEMPLATE: %s joined group %s", peerID, groupID)
	}
}

func (h *Handler) OnLeave(groupID, peerID string, isHost bool) {
	if !isHost {
		log.Printf("TEMPLATE: %s left group %s", peerID, groupID)
	}
}

func (h *Handler) OnClose(groupID string) {
	log.Printf("TEMPLATE: Group %s closed", groupID)
}

func (h *Handler) OnEvent(_ *group.Event) {}
