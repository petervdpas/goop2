package files

import (
	"log"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
)

// Handler implements group.TypeHandler for the "files" group_type.
type Handler struct {
	mqMgr *mq.Manager
	store *Store
}

// New creates a files handler and registers it with the group manager.
func New(mqMgr *mq.Manager, grpMgr *group.Manager, store *Store) {
	h := &Handler{
		mqMgr: mqMgr,
		store: store,
	}
	grpMgr.RegisterType("files", h)
}

func (h *Handler) Flags() group.GroupTypeFlags {
	return group.GroupTypeFlags{HostCanJoin: true}
}

func (h *Handler) OnCreate(_, _ string, _ int) error { return nil }
func (h *Handler) OnJoin(_, _ string, _ bool)                {}
func (h *Handler) OnLeave(_, _ string, _ bool)               {}
func (h *Handler) OnEvent(_ *group.Event)                    {}

func (h *Handler) OnClose(groupID string) {
	log.Printf("FILES: Group %s closed", groupID)
}
