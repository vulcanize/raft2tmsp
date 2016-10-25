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
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"github.com/coreos/etcd/raft"
	pb "github.com/coreos/etcd/raft/raftpb"

	. "github.com/tendermint/go-common"
	cfg "github.com/tendermint/go-config"
	//"github.com/tendermint/go-logger"
	"github.com/tendermint/go-p2p"
	rpcclient "github.com/tendermint/go-rpc/client"

	tmcfg "github.com/tendermint/tendermint/config/tendermint"
	tmnode "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"

	"golang.org/x/net/context"
)

var (
	emptyState = pb.HardState{}

	// ErrStopped is returned by methods on Nodes that have been stopped.
	ErrStopped = errors.New("raft: stopped")
)

func init_tm_files(c cfg.Config) {
	privValidator := tmtypes.GenPrivValidator()
	privValidator.SetFile(c.GetString("priv_validator_file"))
	privValidator.Save()

	genDoc := tmtypes.GenesisDoc{
		ChainID: "chain",
	}
	genDoc.Validators = []tmtypes.GenesisValidator{tmtypes.GenesisValidator{
		PubKey: privValidator.PubKey,
		Amount: 10,
	}}

	genDoc.SaveAs(c.GetString("genesis_file"))
}

// NOTE: this is totally unsafe.
// it's only suitable for testnets.
func reset_all(config cfg.Config) {
	reset_priv_validator(config)
	os.RemoveAll(config.GetString("db_dir"))
	os.Remove(config.GetString("cswal"))
}

// NOTE: this is totally unsafe.
// it's only suitable for testnets.
func reset_priv_validator(config cfg.Config) {
	// Get PrivValidator
	var privValidator *tmtypes.PrivValidator
	privValidatorFile := config.GetString("priv_validator_file")
	if _, err := os.Stat(privValidatorFile); err == nil {
		privValidator = tmtypes.LoadPrivValidator(privValidatorFile)
		privValidator.Reset()
	} else {
		privValidator = tmtypes.GenPrivValidator()
		privValidator.SetFile(privValidatorFile)
		privValidator.Save()
	}
}

func containsUpdates(rd raft.Ready) bool {
	return rd.SoftState != nil || !raft.IsEmptyHardState(rd.HardState) ||
		!raft.IsEmptySnap(rd.Snapshot) || len(rd.Entries) > 0 ||
		len(rd.CommittedEntries) > 0 || len(rd.Messages) > 0 || rd.Index != raft.None
}

func RunTMNode(config cfg.Config) (*tmnode.Node, net.Listener) {
	// Wait until the genesis doc becomes available
	genDocFile := config.GetString("genesis_file")
	if !FileExists(genDocFile) {
		//log.Notice(Fmt("Waiting for genesis file %v...", genDocFile))
		for {
			time.Sleep(time.Second)
			if !FileExists(genDocFile) {
				continue
			}
			jsonBlob, err := ioutil.ReadFile(genDocFile)
			if err != nil {
				Exit(Fmt("Couldn't read GenesisDoc file: %v", err))
			}
			genDoc := tmtypes.GenesisDocFromJSON(jsonBlob)
			if genDoc.ChainID == "" {
				PanicSanity(Fmt("Genesis doc %v must include non-empty chain_id", genDocFile))
			}
			config.Set("chain_id", genDoc.ChainID)
		}
	}

	// Create & start node
	n := tmnode.NewNodeDefault(config)

	protocol, address := tmnode.ProtocolAndAddress(config.GetString("node_laddr"))
	l := p2p.NewDefaultListener(protocol, address, config.GetBool("skip_upnp"))
	n.AddListener(l)
	err := n.Start()
	if err != nil {
		Exit(Fmt("Failed to start node: %v", err))
	}

	//log.Notice("Started node", "nodeInfo", n.sw.NodeInfo())

	// If seedNode is provided by config, dial out.
	if config.GetString("seeds") != "" {
		seeds := strings.Split(config.GetString("seeds"), ",")
		n.DialSeeds(seeds)
	}

	// Run the RPC server.
	var rpcl []net.Listener
	if config.GetString("rpc_laddr") != "" {
		rpcl, err = n.StartRPC()
		if err != nil {
			PanicCrisis(err)
		}
	}

	return n, rpcl[0]
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

	tmrpcclient *rpcclient.ClientURI

	tnode *tmnode.Node
	httprpcl net.Listener
}

func newNode(c cfg.Config) node {
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
		tmrpcclient: rpcclient.NewClientURI(c.GetString("rpc_laddr")),
	}
}

func (n *node) run(index uint64, prevterm uint64, initRD raft.Ready) {
	//logger.SetLogLevel("error")

	var propc chan pb.Message
	var readyc chan raft.Ready
	var rd raft.Ready = initRD
	var advancec chan struct{}
	var prevSoftSt *raft.SoftState
	prevHardSt := emptyState
	prevTerm := prevterm

	var lastHeight int
	var lastIndex uint64 = index

	for {
		if advancec != nil {
			readyc = nil
		} else if !containsUpdates(rd) {
			var r core_types.TMResult

			_, _ = n.tmrpcclient.Call("block", map[string]interface{}{"height": lastHeight + 1}, &r)

			if r != nil {
				lastHeight++
				res := r.(*core_types.ResultBlock)
				tx_num := res.BlockMeta.Header.NumTxs

				if tx_num > 0 {
					rd = raft.Ready{}
					rd.SoftState = prevSoftSt
					rd.HardState = prevHardSt
					for i, tx := range res.Block.Data.Txs {
						entry := pb.Entry{Data: tx, Type: pb.EntryNormal, Index: lastIndex + uint64(i+1),
							Term: prevTerm}
						rd.Entries = append(rd.Entries, entry)
						rd.CommittedEntries = append(rd.CommittedEntries, entry)
					}
					lastIndex += uint64(res.BlockMeta.Header.NumTxs)

					readyc = n.readyc
				} else {
					readyc = nil
				}
			} else {
				readyc = nil
			}
		} else {
			readyc = n.readyc
		}

		propc = n.propc

		select {
		case m := <-propc:
		/*
		rnd, _ := rand.Int(rand.Reader, big.NewInt(256))
		for _, b := range rnd.Bytes() {
			data = append(data, b)
		}
		*/
			var r core_types.TMResult

			data := m.Entries[0].Data
			_, err := n.tmrpcclient.Call("broadcast_tx_commit", map[string]interface{}{"tx": data}, &r)

			if err != nil {
				n.logger.Error(err)
			}

		case m := <-n.recvc:
			if m.Type == pb.MsgHup {
				prevTerm++
			}

		/*
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
		*/
		case <-n.tickc:

		case readyc <- rd:
			rd = raft.Ready{}
			advancec = n.advancec
		case <-advancec:
		/*
		if prevHardSt.Commit != 0 {
			r.raftLog.appliedTo(prevHardSt.Commit)
		}
		if havePrevLastUnstablei {
			r.raftLog.stableTo(prevLastUnstablei, prevLastUnstablet)
			havePrevLastUnstablei = false
		}
		r.raftLog.stableSnapTo(prevSnapi)
		*/
			advancec = nil
		/*
		case c := <-n.status:
			c <- getStatus(r)
		*/
		case <-n.stop:
			n.tnode.Stop()
			n.httprpcl.Close()
			close(n.done)
			return

		default:

		}
	}
}

// StartNode returns a new Node given configuration and a list of raft peers.
// It appends a ConfChangeAddNode entry for each given peer to the initial log.
func StartNode(c *raft.Config, peers []raft.Peer) raft.Node {
	/* Run a tendermint Node */

	config := tmcfg.GetConfig(fmt.Sprintf(".tendermint/node_%v", c.ID))
	config.Set("node_id", c.ID)
	config.Set("node_laddr", fmt.Sprintf("tcp://0.0.0.0:%v", 46659 + c.ID))
	config.Set("rpc_laddr", fmt.Sprintf("tcp://0.0.0.0:%v", 46675 + c.ID))
	config.Set("proxy_app", "nilapp")

	index := uint64(0)
	confChangeEntries := []pb.Entry{}
	seeds := []string{}
	for _, peer := range peers {
		cc := pb.ConfChange{Type: pb.ConfChangeAddNode, NodeID: peer.ID, Context: peer.Context}
		d, err := cc.Marshal()
		if err != nil {
			panic("unexpected marshal error")
		}
		e := pb.Entry{Type: pb.EntryConfChange, Term: 1, Index: index + 1, Data: d}

		confChangeEntries = append(confChangeEntries, e)
		index++

		if peer.ID != c.ID {
			seeds = append(seeds, fmt.Sprintf("0.0.0.0:%v", 46655 + peer.ID))
		}
	}
	config.Set("seeds", strings.Join(seeds, ","))

	initRD := raft.Ready{Entries: confChangeEntries, CommittedEntries: confChangeEntries}

	reset_all(config)
	init_tm_files(config)

	/* Create a raft2tmsp Node */
	n := newNode(config)
	n.logger = c.Logger
	n.tnode, n.httprpcl = RunTMNode(config)

	go n.run(index, uint64(1), initRD)
	return &n
}

// RestartNode is similar to StartNode but does not take a list of peers.
// The current membership of the cluster will be restored from the Storage.
// If the caller has an existing state machine, pass in the last log index that
// has been applied to it; otherwise use zero.
func RestartNode(c *raft.Config) raft.Node {
	/* Run a tendermint Node */

	config := tmcfg.GetConfig(fmt.Sprintf(".tendermint/node_%v", c.ID))
	config.Set("node_id", c.ID)
	config.Set("node_laddr", fmt.Sprintf("tcp://0.0.0.0:%v", 46659 + c.ID))
	config.Set("rpc_laddr", fmt.Sprintf("tcp://0.0.0.0:%v", 46675 + c.ID))
	config.Set("proxy_app", "nilapp")

	hardState, confState, _ := c.Storage.InitialState()
	prevTerm := hardState.Term
	commitedEntries, _ := c.Storage.Entries(1, hardState.Commit+1, 0)

	seeds := []string{}
	for _, peerID := range confState.Nodes {
		if peerID != c.ID {
			seeds = append(seeds, fmt.Sprintf("0.0.0.0:%v", 46655 + peerID))
		}
	}
	config.Set("seeds", strings.Join(seeds, ","))

	initRD := raft.Ready{CommittedEntries: commitedEntries}

	init_tm_files(config)

	/* Create a raft2tmsp Node */
	n := newNode(config)
	n.logger = c.Logger
	n.tnode, n.httprpcl = RunTMNode(config)

	go n.run(1, prevTerm, initRD)
	return &n
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

// Tick increments the internal logical clock for this Node. Election timeouts
// and heartbeat timeouts are in units of ticks.
func (n *node) Tick() {
	select {
	case n.tickc <- struct{}{}:
	case <-n.done:
	default:
		n.logger.Warningf("A tick missed to fire. Node blocks too long!")
	}
}

func (n *node) Campaign(ctx context.Context) error { return n.step(ctx, pb.Message{Type: pb.MsgHup}) }

func (n *node) Propose(ctx context.Context, data []byte) error {
	return n.step(ctx, pb.Message{Type: pb.MsgProp, Entries: []pb.Entry{{Data: data}}})
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
	select {
	case n.advancec <- struct{}{}:
	case <-n.done:
	}
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
