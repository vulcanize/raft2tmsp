// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package raft2tmsp

import (
	"errors"

	"github.com/coreos/etcd/raft"
	pb "github.com/coreos/etcd/raft/raftpb"

	. "github.com/tendermint/go-common"
	rpcclient "github.com/tendermint/go-rpc/client"
	"github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tmsp/types"
	"github.com/tendermint/tmsp/example/dummy"
	"github.com/tendermint/tmsp/server"

	"golang.org/x/net/context"
)

var (
	emptyState = pb.HardState{}

	// ErrStopped is returned by methods on Nodes that have been stopped.
	ErrStopped = errors.New("raft: stopped")
)

var tmspServer Service
var RPCClient *rpcclient.ClientJSONRPC

func runTMSPServer() {
	// Start the listener
	var err error
	tmspServer, err = server.NewServer("tcp://0.0.0.0:46658", "socket", dummy.NewDummyApplication())
	if err != nil {
		Exit(err.Error())
	}
}

func runRPCClient() {
	// Start the rpc client
	RPCClient = rpcclient.NewClientJSONRPC("tcp://0.0.0.0:46657")
}

// StartNode returns a new Node given configuration and a list of raft peers.
// It appends a ConfChangeAddNode entry for each given peer to the initial log.
func StartNode(c *raft.Config, peers []raft.Peer) raft.Node {
	/*
	r := newRaft(c)
	// become the follower at term 1 and apply initial configuration
	// entries of term 1
	r.becomeFollower(1, None)
	for _, peer := range peers {
		cc := pb.ConfChange{Type: pb.ConfChangeAddNode, NodeID: peer.ID, Context: peer.Context}
		d, err := cc.Marshal()
		if err != nil {
			panic("unexpected marshal error")
		}
		e := pb.Entry{Type: pb.EntryConfChange, Term: 1, Index: r.raftLog.lastIndex() + 1, Data: d}
		r.raftLog.append(e)
	}
	// Mark these initial entries as committed.
	// TODO(bdarnell): These entries are still unstable; do we need to preserve
	// the invariant that committed < unstable?
	r.raftLog.committed = r.raftLog.lastIndex()
	// Now apply them, mainly so that the application can call Campaign
	// immediately after StartNode in tests. Note that these nodes will
	// be added to raft twice: here and when the application's Ready
	// loop calls ApplyConfChange. The calls to addNode must come after
	// all calls to raftLog.append so progress.next is set after these
	// bootstrapping entries (it is an error if we try to append these
	// entries since they have already been committed).
	// We do not set raftLog.applied so the application will be able
	// to observe all conf changes via Ready.CommittedEntries.
	for _, peer := range peers {
		r.addNode(peer.ID)
	}
	*/

//	if tmspServer == nil {
		/* Start the TMSP server */
//		runTMSPServer()
//	}


	if RPCClient == nil {
		/* Start the RPC client */
		runRPCClient()
	}

	n := newNode()
	n.logger = c.Logger
	//go n.run()
	return &n
}

// RestartNode is similar to StartNode but does not take a list of peers.
// The current membership of the cluster will be restored from the Storage.
// If the caller has an existing state machine, pass in the last log index that
// has been applied to it; otherwise use zero.
func RestartNode(c *raft.Config) raft.Node {

	//r := newRaft(c)

//	if tmspServer == nil {
//		/* Start the TMSP server */
//		runTMSPServer()
//	}

	if RPCClient == nil {
		/* Start the RPC client */
		runRPCClient()
	}

	n := newNode()
	n.logger = c.Logger
	//go n.run()
	return &n
}

// node is the canonical implementation of the Node interface
type node struct {
	propc      chan pb.Message
	recvc      chan pb.Message
	confc      chan pb.ConfChange
	confstatec chan pb.ConfState
	readyc     chan raft.Ready
	advancec   chan struct{}
	tickc      chan struct{}
	done       chan struct{}
	stop       chan struct{}
	status     chan chan raft.Status

	logger raft.Logger
}

func newNode() node {
	return node{
		propc:      make(chan pb.Message),
		recvc:      make(chan pb.Message),
		confc:      make(chan pb.ConfChange),
		confstatec: make(chan pb.ConfState),
		readyc:     make(chan raft.Ready),
		advancec:   make(chan struct{}),
		// make tickc a buffered chan, so raft node can buffer some ticks when the node
		// is busy processing raft messages. Raft node will resume process buffered
		// ticks when it becomes idle.
		tickc:  make(chan struct{}, 128),
		done:   make(chan struct{}),
		stop:   make(chan struct{}),
		status: make(chan chan raft.Status),
	}
}

func (n *node) Stop() {
	select {
	case n.stop <- struct{}{}:
		// Not already stopped, so trigger it
	case <-n.done:
		// Node has already been stopped - no need to do anything
		return
	}
	// Block until the stop has been acknowledged by run()
	<-n.done
}

func (n *node) run() {
	/*
	var propc chan pb.Message
	var readyc chan Ready
	var advancec chan struct{}
	var prevLastUnstablei, prevLastUnstablet uint64
	var havePrevLastUnstablei bool
	var prevSnapi uint64
	var rd Ready

	lead := None
	prevSoftSt := r.softState()
	prevHardSt := emptyState
	*/

	for {
		/*
		if advancec != nil {
			readyc = nil
		} else {
			rd = newReady(r, prevSoftSt, prevHardSt)
			if rd.containsUpdates() {
				readyc = n.readyc
			} else {
				readyc = nil
			}
		}

		if lead != r.lead {
			if r.hasLeader() {
				if lead == None {
					r.logger.Infof("raft.node: %x elected leader %x at term %d", r.id, r.lead, r.Term)
				} else {
					r.logger.Infof("raft.node: %x changed leader from %x to %x at term %d", r.id, lead, r.lead, r.Term)
				}
				propc = n.propc
			} else {
				r.logger.Infof("raft.node: %x lost leader %x at term %d", r.id, lead, r.Term)
				propc = nil
			}
			lead = r.lead
		}
		*/
		select {
		// TODO: maybe buffer the config propose if there exists one (the way
		// described in raft dissertation)
		// Currently it is dropped in Step silently.
		/*
		case m := <-n.propc:
			m.From = r.id
			r.Step(m)
		case m := <-n.recvc:
			// filter out response message from unknown From.
			if _, ok := r.prs[m.From]; ok || !IsResponseMsg(m.Type) {
				r.Step(m) // raft never returns an error
			}
		case cc := <-n.confc:
			if cc.NodeID == None {
				r.resetPendingConf()
				select {
				case n.confstatec <- pb.ConfState{Nodes: r.nodes()}:
				case <-n.done:
				}
				break
			}
			switch cc.Type {
			case pb.ConfChangeAddNode:
				r.addNode(cc.NodeID)
			case pb.ConfChangeRemoveNode:
				// block incoming proposal when local node is
				// removed
				if cc.NodeID == r.id {
					propc = nil
				}
				r.removeNode(cc.NodeID)
			case pb.ConfChangeUpdateNode:
				r.resetPendingConf()
			default:
				panic("unexpected conf type")
			}
			select {
			case n.confstatec <- pb.ConfState{Nodes: r.nodes()}:
			case <-n.done:
			}

		case <-n.tickc:

		case readyc <- rd:
			if rd.SoftState != nil {
				prevSoftSt = rd.SoftState
			}
			if len(rd.Entries) > 0 {
				prevLastUnstablei = rd.Entries[len(rd.Entries)-1].Index
				prevLastUnstablet = rd.Entries[len(rd.Entries)-1].Term
				havePrevLastUnstablei = true
			}
			if !IsEmptyHardState(rd.HardState) {
				prevHardSt = rd.HardState
			}
			if !IsEmptySnap(rd.Snapshot) {
				prevSnapi = rd.Snapshot.Metadata.Index
			}

			r.msgs = nil
			r.readState.Index = None
			r.readState.RequestCtx = nil
			advancec = n.advancec
		case <-advancec:
			if prevHardSt.Commit != 0 {
				r.raftLog.appliedTo(prevHardSt.Commit)
			}
			if havePrevLastUnstablei {
				r.raftLog.stableTo(prevLastUnstablei, prevLastUnstablet)
				havePrevLastUnstablei = false
			}
			r.raftLog.stableSnapTo(prevSnapi)
			advancec = nil
		case c := <-n.status:
			c <- getStatus(r)
		case <-n.stop:
			close(n.done)
			return
		*/
		}
	}
}

// Tick increments the internal logical clock for this Node. Election timeouts
// and heartbeat timeouts are in units of ticks.
func (n *node) Tick() {
	/*
	select {
	case n.tickc <- struct{}{}:
	case <-n.done:
	default:
		n.logger.Warningf("A tick missed to fire. Node blocks too long!")
	}
	*/
}

func (n *node) Campaign(ctx context.Context) error { return nil /*return n.step(ctx, pb.Message{Type: pb.MsgHup}) */}

func (n *node) Propose(ctx context.Context, data []byte) error {
	var r core_types.TMResult
	var res *core_types.ResultBroadcastTx
	var err error
	_, err = RPCClient.Call("broadcast_tx_commit", []interface{}{data}, &r)

	if r != nil {
		res = r.(*core_types.ResultBroadcastTx)
		print("code "+res.Code.String()+" data "+string(res.Data)+"\n\n")
		if res.Code == types.CodeType_OK {
			n.readyc <- raft.Ready{
				Entries:          []pb.Entry{},
				CommittedEntries: []pb.Entry{{Data: res.Data, Type: pb.EntryNormal}},
				Messages:         []pb.Message{},
			}
		}
	} else {
		print(err.Error()+"\n")
		return err
	}
	return nil
}

func (n *node) Step(ctx context.Context, m pb.Message) error {
	// ignore unexpected local messages receiving over network
	if raft.IsLocalMsg(m.Type) {
		// TODO: return an error?
		return nil
	}
	return n.step(ctx, m)
}

func (n *node) ProposeConfChange(ctx context.Context, cc pb.ConfChange) error {
	/*
	data, err := cc.Marshal()
	if err != nil {
		return err
	}
	return n.Step(ctx, pb.Message{Type: pb.MsgProp, Entries: []pb.Entry{{Type: pb.EntryConfChange, Data: data}}})
	*/
	return nil
}

// Step advances the state machine using msgs. The ctx.Err() will be returned,
// if any.
func (n *node) step(ctx context.Context, m pb.Message) error {
	ch := n.recvc
	if m.Type == pb.MsgProp {
		ch = n.propc
	}

	select {
	case ch <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-n.done:
		return ErrStopped
	}
}

func (n *node) Ready() <-chan raft.Ready { return n.readyc }

func (n *node) Advance() {
	/*
	select {
	case n.advancec <- struct{}{}:
	case <-n.done:
	}
	*/
}

func (n *node) ApplyConfChange(cc pb.ConfChange) *pb.ConfState {
	/*
	var cs pb.ConfState
	select {
	case n.confc <- cc:
	case <-n.done:
	}
	select {
	case cs = <-n.confstatec:
	case <-n.done:
	}
	return &cs
	*/
	return nil
}

func (n *node) Status() raft.Status {
	/*
	c := make(chan raft.Status)
	n.status <- c
	return <-c
	*/
	return raft.Status{}
}

func (n *node) ReportUnreachable(id uint64) {
	/*
	select {
	case n.recvc <- pb.Message{Type: pb.MsgUnreachable, From: id}:
	case <-n.done:
	}
	*/
}

func (n *node) ReportSnapshot(id uint64, status raft.SnapshotStatus) {
	/*
	rej := status == SnapshotFailure

	select {
	case n.recvc <- pb.Message{Type: pb.MsgSnapStatus, From: id, Reject: rej}:
	case <-n.done:
	}
	*/
}

func (n *node) TransferLeadership(ctx context.Context, lead, transferee uint64) {
	/*
	select {
	// manually set 'from' and 'to', so that leader can voluntarily transfers its leadership
	case n.recvc <- pb.Message{Type: pb.MsgTransferLeader, From: transferee, To: lead}:
	case <-n.done:
	case <-ctx.Done():
	}
	*/
}

func (n *node) ReadIndex(ctx context.Context, rctx []byte) error {
	/*
	return n.step(ctx, pb.Message{Type: pb.MsgReadIndex, Entries: []pb.Entry{{Data: rctx}}})
	*/
	return nil
}
/*
func newReady(r *raft, prevSoftSt *SoftState, prevHardSt pb.HardState) Ready {
	rd := Ready{
		Entries:          r.raftLog.unstableEntries(),
		CommittedEntries: r.raftLog.nextEnts(),
		Messages:         r.msgs,
	}
	if softSt := r.softState(); !softSt.equal(prevSoftSt) {
		rd.SoftState = softSt
	}
	if hardSt := r.hardState(); !isHardStateEqual(hardSt, prevHardSt) {
		rd.HardState = hardSt
	}
	if r.raftLog.unstable.snapshot != nil {
		rd.Snapshot = *r.raftLog.unstable.snapshot
	}
	if r.readState.Index != None {
		c := make([]byte, len(r.readState.RequestCtx))
		copy(c, r.readState.RequestCtx)

		rd.Index = r.readState.Index
		rd.RequestCtx = c
	}
	return rd
}
*/
