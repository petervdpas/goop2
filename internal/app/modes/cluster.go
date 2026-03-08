package modes

import (
	"context"

	"github.com/petervdpas/goop2/internal/cluster"
	"github.com/petervdpas/goop2/internal/group"
	"github.com/petervdpas/goop2/internal/mq"
)

// mqClusterAdapter bridges *mq.Manager to cluster.SendFunc / cluster.SubscribeFunc
// and implements group.TypeHandler to forward lifecycle events.
type mqClusterAdapter struct {
	mqMgr      *mq.Manager
	clusterMgr *cluster.Manager
}

func (a *mqClusterAdapter) send(peerID, topic string, payload any) error {
	ctx, cancel := context.WithTimeout(context.Background(), MQClusterSendTimeout)
	defer cancel()
	_, err := a.mqMgr.Send(ctx, peerID, topic, payload)
	return err
}

func (a *mqClusterAdapter) subscribe(fn func(from, topic string, payload any)) func() {
	return a.mqMgr.SubscribeTopic("cluster:", func(from, topic string, payload any) {
		fn(from, topic, payload)
	})
}

func (a *mqClusterAdapter) OnCreate(_, _ string, _ int, _ bool) error { return nil }
func (a *mqClusterAdapter) OnJoin(_, _ string, _ *group.WelcomePayload) error { return nil }
func (a *mqClusterAdapter) OnLeave(_, _ string) {}
func (a *mqClusterAdapter) OnClose(_ string) {}

func (a *mqClusterAdapter) OnEvent(evt *group.Event) {
	a.clusterMgr.HandleGroupEvent(&cluster.GroupEvent{
		Type:    evt.Type,
		Group:   evt.Group,
		From:    evt.From,
		Payload: evt.Payload,
	})
}

func setupCluster(mqMgr *mq.Manager, grpMgr *group.Manager, selfID string) (*cluster.Manager, *mqClusterAdapter) {
	adapter := &mqClusterAdapter{mqMgr: mqMgr}

	clusterMgr := cluster.New(selfID, adapter.send, adapter.subscribe)
	adapter.clusterMgr = clusterMgr

	grpMgr.RegisterType("cluster", adapter)

	return clusterMgr, adapter
}
