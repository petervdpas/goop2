package cluster

import (
	"context"

	coreCluster "github.com/petervdpas/goop2/internal/cluster"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
)

var sendTimeout = group.ClusterSendTimeout

// Handler implements group.TypeHandler for the "cluster" app_type.
// It bridges MQ messaging and group events to the cluster manager.
type Handler struct {
	mqMgr      *mq.Manager
	clusterMgr *coreCluster.Manager
}

// New creates a cluster handler, wires the MQ adapter, and registers
// the type handler with the group manager.
func New(mqMgr *mq.Manager, grpMgr *group.Manager, selfID string) *coreCluster.Manager {
	h := &Handler{mqMgr: mqMgr}

	clusterMgr := coreCluster.New(selfID, h.send, h.subscribe)
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

func (h *Handler) OnCreate(_, _ string, _ int, _ bool) error { return nil }

func (h *Handler) OnJoin(groupID, _ string, _ *group.WelcomePayload) error {
	if h.clusterMgr.Role() == "" {
		return h.clusterMgr.JoinCluster(groupID)
	}
	return nil
}

func (h *Handler) OnLeave(_, _ string) {
	h.clusterMgr.LeaveCluster()
}

func (h *Handler) OnClose(_ string) {
	h.clusterMgr.LeaveCluster()
}

func (h *Handler) OnEvent(evt *group.Event) {
	h.clusterMgr.HandleGroupEvent(&coreCluster.GroupEvent{
		Type:    evt.Type,
		Group:   evt.Group,
		From:    evt.From,
		Payload: evt.Payload,
	})
}
