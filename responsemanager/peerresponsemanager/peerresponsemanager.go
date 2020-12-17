package peerresponsemanager

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/ipfs/go-graphsync/peermanager"
)

// PeerSenderFactory provides a function that will create a PeerResponseSender.
type PeerSenderFactory func(ctx context.Context, p peer.ID) PeerResponseBuilder

// PeerResponseManager manages message queues for peers
type PeerResponseManager struct {
	*peermanager.PeerManager
}

// New generates a new peer manager for sending responses
func New(ctx context.Context, createPeerSender PeerSenderFactory) *PeerResponseManager {
	return &PeerResponseManager{
		PeerManager: peermanager.New(ctx, func(ctx context.Context, p peer.ID) peermanager.PeerHandler {
			return createPeerSender(ctx, p)
		}),
	}
}

// SenderForPeer returns a response sender to use with the given peer
func (prm *PeerResponseManager) SenderForPeer(p peer.ID) PeerResponseBuilder {
	return prm.GetProcess(p).(PeerResponseBuilder)
}
