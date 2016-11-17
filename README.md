#Translator from Raft to TMSP protocols

### Raft state machine

Raft parts:
- log - ordered sequence of entries
- state maching
- peers
- quorum - (n/2 +1)
- commited log entry - entry approved by quorum

There are 4 states:
- Follower
- PreCandidate
- Candidate
- Leader

Basic transition: Follower -> (PreCandidate) -> Candidate -> Leader

PreCandidate is optional

There are two big different data type:
- [raft](https://github.com/coreos/etcd/blob/master/raft/raft.go#L209)
- [node](https://github.com/coreos/etcd/blob/master/raft/node.go#L224)

`raft` is core, state machine. `node` consists of couple channels. Channel for outgoing messages - `propc` (propose), incoming - `recvc`, configuration etc.
`node` handles any messages from channels and pass to `raft`
so `node` is on higher abstract layer that `raft`

When `raft` started it has `state=StateFollower` and change `step` function to [`stepFollower`](https://github.com/coreos/etcd/blob/master/raft/raft.go#L1024)
`step` function is core of state machine. it processes all messages

Then we call `node.Campaign()` (causes node to transition to Candidate state) then `node` pass message type `MsgHup` to `raft` and raft's Step function handle it and transite `raft` to Candidate state.

`raft` node vote for itself and it gets `MsgVoteResp` and becomes leader.

### Types of Raft message

- `MsgHup` - sent from follower and canditate to start campaing to become leader
- `MsgBeat` -  run by leaders to send heartbeat (edited)
- `MsgProp` - send by follower or leader (not candidate). propose data to be added to log 
- `msgApp` message with entries from leader to candidate and follower.  
If `Index` in incoming message less than highest node log position node approve this message (send message type `MsgAppResp`) but don't append it because it already appended.  
If index higher, then node check could it append to it log. If yes - it send `MsgAppResp` with new Index, otherwise it send message type `MsgAppResp` but with flag `Reject: true`
- `MsgAppResp` - response for `MsgAppResp`
- `MsgVote` - vote for leader
- `MsgVoteResp` - response for vote
- `MsgSnap` - snapshot from leader. response for this message is `MsgAppResp`
- `MsgHeartbeat` - heartbeat. response - `MsgHeartbeatResp`
- `MsgUnreachable` - remote becomes unreachable
- `MsgSnapStatus` - response for snapshot message (edited)
- `MsgCheckQuorum` tell the leader to check quorum. if quorum is not active leader becomes follower
- `MsgTransferLeader`- transfer leader to new node
- `MsgTimeoutNow` - message to follower to start campaign transfer
- `MsgReadIndex` request to leader to get highest log position
- `MsgReadIndexResp`response for `MsgReadIndex`