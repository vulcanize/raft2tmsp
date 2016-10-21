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
	//"bytes"
	//"fmt"
	"reflect"
	"math"
	"testing"
	"time"

	//"github.com/coreos/etcd/pkg/testutil"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/raft"

	"github.com/tendermint/go-logger"

	"golang.org/x/net/context"
)

const noLimit = math.MaxUint64

// TestNodeStep ensures that node.Step sends msgProp to propc chan
// and other kinds of messages to recvc chan.
func TestNodeStep(t *testing.T) {
	for i, msgn := range raftpb.MessageType_name {
		n := &node{
			propc: make(chan raftpb.Message, 1),
			recvc: make(chan raftpb.Message, 1),
		}
		msgt := raftpb.MessageType(i)
		n.Step(context.TODO(), raftpb.Message{Type: msgt})
		// Proposal goes to proc chan. Others go to recvc chan.
		if msgt == raftpb.MsgProp {
			select {
			case <-n.propc:
			default:
				t.Errorf("%d: cannot receive %s on propc chan", msgt, msgn)
			}
		} else {
			if raft.IsLocalMsg(msgt) {
				select {
				case <-n.recvc:
					t.Errorf("%d: step should ignore %s", msgt, msgn)
				default:
				}
			} else {
				select {
				case <-n.recvc:
				default:
					t.Errorf("%d: cannot receive %s on recvc chan", msgt, msgn)
				}
			}
		}
	}
}

// Cancel and Stop should unblock Step()
func TestNodeStepUnblock(t *testing.T) {
	// a node without buffer to block step
	n := &node{
		propc: make(chan raftpb.Message),
		done:  make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopFunc := func() { close(n.done) }

	tests := []struct {
		unblock func()
		werr    error
	}{
		{stopFunc, ErrStopped},
		{cancel, context.Canceled},
	}

	for i, tt := range tests {
		errc := make(chan error, 1)
		go func() {
			err := n.Step(ctx, raftpb.Message{Type: raftpb.MsgProp})
			errc <- err
		}()
		tt.unblock()
		select {
		case err := <-errc:
			if err != tt.werr {
				t.Errorf("#%d: err = %v, want %v", i, err, tt.werr)
			}
			//clean up side-effect
			if ctx.Err() != nil {
				ctx = context.TODO()
			}
			select {
			case <-n.done:
				n.done = make(chan struct{})
			default:
			}
		case <-time.After(1 * time.Second):
			t.Errorf("#%d: failed to unblock step", i)
		}
	}
}

func TestReadyContainUpdates(t *testing.T) {
	tests := []struct {
		rd       raft.Ready
		wcontain bool
	}{
		{raft.Ready{}, false},
		{raft.Ready{SoftState: &raft.SoftState{Lead: 1}}, true},
		{raft.Ready{HardState: raftpb.HardState{Vote: 1}}, true},
		{raft.Ready{Entries: make([]raftpb.Entry, 1, 1)}, true},
		{raft.Ready{CommittedEntries: make([]raftpb.Entry, 1, 1)}, true},
		{raft.Ready{Messages: make([]raftpb.Message, 1, 1)}, true},
		{raft.Ready{Snapshot: raftpb.Snapshot{Metadata: raftpb.SnapshotMetadata{Index: 1}}}, true},
	}

	for i, tt := range tests {
		if g := containsUpdates(tt.rd); g != tt.wcontain {
			t.Errorf("#%d: containUpdates = %v, want %v", i, g, tt.wcontain)
		}
	}
}

// TestNodeStart ensures that a node can be started correctly. The node should
// start with correct configuration change entries, and can accept and commit
// proposals.
func TestNodeStart(t *testing.T) {
	logger.SetLogLevel("error")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cc := raftpb.ConfChange{Type: raftpb.ConfChangeAddNode, NodeID: 1}
	ccdata, err := cc.Marshal()
	if err != nil {
		t.Errorf("unexpected marshal error: %v", err)
	}
	wants := []raft.Ready{
		{
			//HardState: raftpb.HardState{Term: 1, Commit: 1, Vote: 0},
			Entries: []raftpb.Entry{
				{Type: raftpb.EntryConfChange, Term: 1, Index: 1, Data: ccdata},
			},
			CommittedEntries: []raftpb.Entry{
				{Type: raftpb.EntryConfChange, Term: 1, Index: 1, Data: ccdata},
			},
		},
		{
			//HardState:        raftpb.HardState{Term: 2, Commit: 3, Vote: 1},
			Entries:          []raftpb.Entry{{Term: 2, Index: 2, Data: []byte("foo")}},
			CommittedEntries: []raftpb.Entry{{Term: 2, Index: 2, Data: []byte("foo")}},
		},
	}
	storage := raft.NewMemoryStorage()
	c := &raft.Config{
		ID:              1,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         storage,
		MaxSizePerMsg:   noLimit,
		MaxInflightMsgs: 256,
	}
	n := StartNode(c, []raft.Peer{{ID: 1}})
	defer n.Stop()

	g := <-n.Ready()
	if !reflect.DeepEqual(g, wants[0]) {
		t.Errorf("#%d: g = %+v,\n             w   %+v", 1, g, wants[0])
	} else {
		storage.Append(g.Entries)
		n.Advance()
	}

	n.Campaign(ctx)
	//rd := <-n.Ready()
	//storage.Append(rd.Entries)
	//n.Advance()

	n.Propose(ctx, []byte("foo"))
	if g2 := <-n.Ready(); !reflect.DeepEqual(g2, wants[1]) {
		t.Errorf("#%d: g = %+v,\n             w   %+v", 2, g2, wants[1])
	} else {
		storage.Append(g2.Entries)
		n.Advance()
	}

	select {
	case rd := <-n.Ready():
		t.Errorf("unexpected Ready: %+v", rd)
	case <-time.After(time.Millisecond):
	}
}
/*
func TestNodeRestart(t *testing.T) {
	entries := []raftpb.Entry{
		{Term: 1, Index: 1},
		{Term: 1, Index: 2, Data: []byte("foo")},
	}
	st := raftpb.HardState{Term: 1, Commit: 1}

	want := Ready{
		HardState: st,
		// commit up to index commit index in st
		CommittedEntries: entries[:st.Commit],
	}

	storage := NewMemoryStorage()
	storage.SetHardState(st)
	storage.Append(entries)
	c := &Config{
		ID:              1,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         storage,
		MaxSizePerMsg:   noLimit,
		MaxInflightMsgs: 256,
	}
	n := RestartNode(c)
	defer n.Stop()
	if g := <-n.Ready(); !reflect.DeepEqual(g, want) {
		t.Errorf("g = %+v,\n             w   %+v", g, want)
	}
	n.Advance()

	select {
	case rd := <-n.Ready():
		t.Errorf("unexpected Ready: %+v", rd)
	case <-time.After(time.Millisecond):
	}
}

func TestNodeRestartFromSnapshot(t *testing.T) {
	snap := raftpb.Snapshot{
		Metadata: raftpb.SnapshotMetadata{
			ConfState: raftpb.ConfState{Nodes: []uint64{1, 2}},
			Index:     2,
			Term:      1,
		},
	}
	entries := []raftpb.Entry{
		{Term: 1, Index: 3, Data: []byte("foo")},
	}
	st := raftpb.HardState{Term: 1, Commit: 3}

	want := Ready{
		HardState: st,
		// commit up to index commit index in st
		CommittedEntries: entries,
	}

	s := NewMemoryStorage()
	s.SetHardState(st)
	s.ApplySnapshot(snap)
	s.Append(entries)
	c := &Config{
		ID:              1,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         s,
		MaxSizePerMsg:   noLimit,
		MaxInflightMsgs: 256,
	}
	n := RestartNode(c)
	defer n.Stop()
	if g := <-n.Ready(); !reflect.DeepEqual(g, want) {
		t.Errorf("g = %+v,\n             w   %+v", g, want)
	} else {
		n.Advance()
	}

	select {
	case rd := <-n.Ready():
		t.Errorf("unexpected Ready: %+v", rd)
	case <-time.After(time.Millisecond):
	}
}

func TestNodeAdvance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storage := NewMemoryStorage()
	c := &Config{
		ID:              1,
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         storage,
		MaxSizePerMsg:   noLimit,
		MaxInflightMsgs: 256,
	}
	n := StartNode(c, []Peer{{ID: 1}})
	defer n.Stop()
	rd := <-n.Ready()
	storage.Append(rd.Entries)
	n.Advance()

	n.Campaign(ctx)
	<-n.Ready()

	n.Propose(ctx, []byte("foo"))
	select {
	case rd = <-n.Ready():
		t.Errorf("unexpected Ready before Advance: %+v", rd)
	case <-time.After(time.Millisecond):
	}
	storage.Append(rd.Entries)
	n.Advance()
	select {
	case <-n.Ready():
	case <-time.After(100 * time.Millisecond):
		t.Errorf("expect Ready after Advance, but there is no Ready available")
	}
}

func TestSoftStateEqual(t *testing.T) {
	tests := []struct {
		st *SoftState
		we bool
	}{
		{&SoftState{}, true},
		{&SoftState{Lead: 1}, false},
		{&SoftState{RaftState: StateLeader}, false},
	}
	for i, tt := range tests {
		if g := tt.st.equal(&SoftState{}); g != tt.we {
			t.Errorf("#%d, equal = %v, want %v", i, g, tt.we)
		}
	}
}

func TestIsHardStateEqual(t *testing.T) {
	tests := []struct {
		st raftpb.HardState
		we bool
	}{
		{emptyState, true},
		{raftpb.HardState{Vote: 1}, false},
		{raftpb.HardState{Commit: 1}, false},
		{raftpb.HardState{Term: 1}, false},
	}

	for i, tt := range tests {
		if isHardStateEqual(tt.st, emptyState) != tt.we {
			t.Errorf("#%d, equal = %v, want %v", i, isHardStateEqual(tt.st, emptyState), tt.we)
		}
	}
}
*/