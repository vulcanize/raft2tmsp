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
	"golang.org/x/net/context"

	"github.com/coreos/etcd/raft"
	pb "github.com/coreos/etcd/raft/raftpb"
)

type RawNode struct {
	node raft.Node
}

// NewRawNode returns a new RawNode given configuration and a list of raft peers.
func NewRawNode(config *raft.Config, peers []raft.Peer) (*RawNode, error) {
	rn := RawNode{node: StartNode(config, peers)}

	return &rn, nil
}

// Tick advances the internal logical clock by a single tick.
func (rn *RawNode) Tick() {
	rn.node.Tick()
}

func (rn *RawNode) TickQuiesced() {
	//@TODO implement
}

// Campaign causes this RawNode to transition to candidate state.
func (rn *RawNode) Campaign() error {
	return rn.node.Campaign(context.TODO())
}

// Propose proposes data be appended to the raft log.
func (rn *RawNode) Propose(data []byte) error {
	return rn.node.Propose(context.TODO(), data)
}

// ProposeConfChange proposes a config change.
func (rn *RawNode) ProposeConfChange(cc pb.ConfChange) error {
	return rn.node.ProposeConfChange(context.TODO(), cc)
}

// ApplyConfChange applies a config change to the local node.
func (rn *RawNode) ApplyConfChange(cc pb.ConfChange) *pb.ConfState {
	return rn.node.ApplyConfChange(cc)
}

// Step advances the state machine using the given message.
func (rn *RawNode) Step(m pb.Message) error {
	return rn.node.Step(context.TODO(), m)
}

// Ready returns the current point-in-time state of this RawNode.
func (rn *RawNode) Ready() raft.Ready {
	return rn.node.(*node).GetReady()
}

// HasReady called when RawNode user need to check if any Ready pending.
// Checking logic in this method should be consistent with Ready.containsUpdates().
func (rn *RawNode) HasReady() bool {
	return containsUpdates(rn.node.(*node).GetReady())
}

// Advance notifies the RawNode that the application has applied and saved progress in the
// last Ready results.
func (rn *RawNode) Advance(rd raft.Ready) {
	rn.node.Advance()
}

// Status returns the current status of the given group.
func (rn *RawNode) Status() *raft.Status {
	status := rn.node.Status()
	return &status
}

// ReportUnreachable reports the given node is not reachable for the last send.
func (rn *RawNode) ReportUnreachable(id uint64) {
	rn.node.ReportUnreachable(id)
}

// ReportSnapshot reports the status of the sent snapshot.
func (rn *RawNode) ReportSnapshot(id uint64, status raft.SnapshotStatus) {
	rn.node.ReportSnapshot(id, status)
}

// TransferLeader tries to transfer leadership to the given transferee.
func (rn *RawNode) TransferLeader(transferee uint64) {
	rn.node.Step(context.TODO(), pb.Message{Type: pb.MsgTransferLeader, From: transferee})
}
