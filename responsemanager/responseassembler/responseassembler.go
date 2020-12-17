/*
Package responseassembler assembles responses that are queued for sending in outgoing messages

The response assembler's Transaction method allows a caller to specify response actions that will go into a single
libp2p2 message. The response assembler will also deduplicate blocks that have already been sent over the network in
a previous message
*/
package responseassembler

import (
	"context"

	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/ipfs/go-graphsync"
	gsmsg "github.com/ipfs/go-graphsync/message"
	"github.com/ipfs/go-graphsync/notifications"
	"github.com/ipfs/go-graphsync/peermanager"
)

// Transaction is a series of operations that should be send together in a single response
type Transaction func(TransactionBuilder) error

// TransactionBuilder is a limited interface for assembling responses inside a transaction, so that they are included
// in the same message on the protocol
type TransactionBuilder interface {
	SendResponse(
		link ipld.Link,
		data []byte,
	) graphsync.BlockData
	SendExtensionData(graphsync.ExtensionData)
	FinishWithCancel()
	FinishRequest() graphsync.ResponseStatusCode
	FinishWithError(status graphsync.ResponseStatusCode)
	PauseRequest()
	AddNotifee(notifications.Notifee)
}

// PeerMessageHandler is an interface that can queues a response for a given peer to go out over the network
type PeerMessageHandler interface {
	BuildMessage(p peer.ID, blkSize uint64, buildResponseFn func(*gsmsg.Builder), notifees []notifications.Notifee)
}

// Allocator is an interface that can manage memory allocated for blocks
type Allocator interface {
	AllocateBlockMemory(p peer.ID, amount uint64) <-chan error
}

// ResponseAssembler manages assembling responses to go out over the network
// in libp2p messages
type ResponseAssembler struct {
	*peermanager.PeerManager
	allocator   Allocator
	peerHandler PeerMessageHandler
	ctx         context.Context
}

// New generates a new ResponseAssembler for sending responses
func New(ctx context.Context, allocator Allocator, peerHandler PeerMessageHandler) *ResponseAssembler {
	return &ResponseAssembler{
		PeerManager: peermanager.New(ctx, func(ctx context.Context, p peer.ID) peermanager.PeerHandler {
			return newTracker()
		}),
		ctx:         ctx,
		allocator:   allocator,
		peerHandler: peerHandler,
	}
}

// DedupKey indicates that outgoing blocks should be deduplicated in a seperate bucket (only with requests that share
// supplied key string)
func (ra *ResponseAssembler) DedupKey(p peer.ID, requestID graphsync.RequestID, key string) {
	ra.GetProcess(p).(*peerLinkTracker).DedupKey(requestID, key)
}

// IgnoreBlocks indicates that a list of keys that should be ignored when sending blocks
func (ra *ResponseAssembler) IgnoreBlocks(p peer.ID, requestID graphsync.RequestID, links []ipld.Link) {
	ra.GetProcess(p).(*peerLinkTracker).IgnoreBlocks(requestID, links)
}

// Transaction build a response, and queues it for sending in the next outgoing message
func (ra *ResponseAssembler) Transaction(p peer.ID, requestID graphsync.RequestID, transaction Transaction) error {
	prts := &transactionBuilder{
		requestID:   requestID,
		linkTracker: ra.GetProcess(p).(*peerLinkTracker),
	}
	err := transaction(prts)
	if err == nil {
		ra.execute(p, prts.operations, prts.notifees)
	}
	return err
}

func (ra *ResponseAssembler) execute(p peer.ID, operations []responseOperation, notifees []notifications.Notifee) {
	size := uint64(0)
	for _, op := range operations {
		size += op.size()
	}
	if size > 0 {
		select {
		case <-ra.allocator.AllocateBlockMemory(p, size):
		case <-ra.ctx.Done():
			return
		}
	}
	ra.peerHandler.BuildMessage(p, size, func(responseBuilder *gsmsg.Builder) {
		for _, op := range operations {
			op.build(responseBuilder)
		}
	}, notifees)
}
