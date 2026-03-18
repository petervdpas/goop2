package cluster

import (
	"context"

	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
)

var sendTimeout = group.ClusterSendTimeout

type Handler struct {
	mqMgr      *mq.Manager
	clusterMgr *Manager
}

func New(mqMgr *mq.Manager, grpMgr *group.Manager, selfID string) *Manager {
	h := &Handler{mqMgr: mqMgr}

	clusterMgr := NewManager(selfID, h.send, h.subscribe)
	h.clusterMgr = clusterMgr

	grpMgr.RegisterType("cluster", h)

	return clusterMgr
}

func (h *Handler) send(peerID, topic string, payload any) error {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	_, err := h.mqMgr.Send(ctx, peerID, topic, payload)
	return err
}

func (h *Handler) subscribe(fn func(from, topic string, payload any)) func() {
	return h.mqMgr.SubscribeTopic("cluster:", func(from, topic string, payload any) {
		fn(from, topic, payload)
	})
}

func (h *Handler) Flags() group.TypeFlags {
	return group.TypeFlags{HostCanJoin: false}
}

func (h *Handler) OnCreate(_, _ string, _ int, _ bool) error { return nil }

func (h *Handler) OnJoin(groupID, peerID string, isHost bool) {
	h.clusterMgr.HandleGroupEvent(&GroupEvent{
		Type:  "join",
		Group: groupID,
		From:  peerID,
	})
}

func (h *Handler) OnLeave(groupID, peerID string, isHost bool) {
	h.clusterMgr.HandleGroupEvent(&GroupEvent{
		Type:  "leave",
		Group: groupID,
		From:  peerID,
	})
}

func (h *Handler) OnClose(_ string) {
	h.clusterMgr.LeaveCluster()
}

func (h *Handler) OnEvent(evt *group.Event) {
	switch {
	case evt.Type == "welcome" && h.clusterMgr.Role() == "":
		_ = h.clusterMgr.JoinCluster(evt.Group, evt.From)
	case evt.Type == "welcome" && h.clusterMgr.Role() == "worker":
		_ = h.clusterMgr.JoinCluster(evt.Group, evt.From)
	case h.clusterMgr.Role() == "worker" && evt.Type == "leave":
		h.clusterMgr.LeaveCluster()
	case h.clusterMgr.Role() == "worker" && evt.Type == "close":
		h.clusterMgr.LeaveCluster()
	}
	h.clusterMgr.HandleGroupEvent(&GroupEvent{
		Type:    evt.Type,
		Group:   evt.Group,
		From:    evt.From,
		Payload: evt.Payload,
	})
}
