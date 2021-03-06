// Copyright 2019-present Open Networking Foundation.
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

package storage

import (
	"context"
	"fmt"
	"github.com/atomix/atomix-go-framework/pkg/atomix/cluster"
	"github.com/atomix/atomix-go-framework/pkg/atomix/errors"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	protocol "github.com/atomix/atomix-go-framework/pkg/atomix/storage/protocol/rsm"
	"github.com/atomix/atomix-raft-storage/pkg/storage/config"
	"github.com/lni/dragonboat/v3"
	raftconfig "github.com/lni/dragonboat/v3/config"
	"github.com/lni/dragonboat/v3/statemachine"
	"sort"
	"sync"
)

const dataDir = "/var/lib/atomix/data"
const rttMillisecond = 200

var log = logging.GetLogger("atomix", "raft")

// NewProtocol returns a new Raft Protocol instance
func NewProtocol(config config.ProtocolConfig) *Protocol {
	protocol := &Protocol{
		config:  config,
		clients: make(map[protocol.PartitionID]*Partition),
		servers: make(map[protocol.PartitionID]*Server),
	}
	protocol.listener = &raftEventListener{
		protocol:  protocol,
		listeners: make(map[int]chan<- RaftEvent),
	}
	return protocol
}

// Protocol is an implementation of the Client interface providing the Raft consensus protocol
type Protocol struct {
	protocol.Protocol
	config          config.ProtocolConfig
	mu              sync.RWMutex
	replicas        []*cluster.Replica
	clients         map[protocol.PartitionID]*Partition
	servers         map[protocol.PartitionID]*Server
	memberIDs       map[uint64]string
	nodeIDs         map[string]uint64
	memberAddresses map[uint64]string
	listener        *raftEventListener
}

func (p *Protocol) watch(ctx context.Context, ch chan<- RaftEvent) {
	p.listener.listen(ctx, ch)
}

func (p *Protocol) getMemberIDs() map[uint64]string {
	p.mu.RLock()
	memberIDs := p.memberIDs
	p.mu.RUnlock()
	if memberIDs != nil {
		return memberIDs
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.memberIDs != nil {
		return p.memberIDs
	}

	p.memberIDs = make(map[uint64]string)
	for i, replica := range p.replicas {
		p.memberIDs[uint64(i+1)] = string(replica.ID)
	}
	return p.memberIDs
}

func (p *Protocol) getMemberID(id uint64) string {
	return p.getMemberIDs()[id]
}

func (p *Protocol) getNodeIDs() map[string]uint64 {
	p.mu.RLock()
	nodeIDs := p.nodeIDs
	p.mu.RUnlock()
	if nodeIDs != nil {
		return nodeIDs
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.nodeIDs != nil {
		return p.nodeIDs
	}

	p.nodeIDs = make(map[string]uint64)
	for i, replica := range p.replicas {
		p.nodeIDs[string(replica.ID)] = uint64(i + 1)
	}
	return p.nodeIDs
}

func (p *Protocol) getNodeID(id string) uint64 {
	return p.getNodeIDs()[id]
}

func (p *Protocol) getAddresses() map[uint64]string {
	p.mu.RLock()
	memberAddresses := p.memberAddresses
	p.mu.RUnlock()
	if memberAddresses != nil {
		return memberAddresses
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.memberAddresses != nil {
		return p.memberAddresses
	}

	p.memberAddresses = make(map[uint64]string)
	for i, replica := range p.replicas {
		p.memberAddresses[uint64(i+1)] = fmt.Sprintf("%s:%d", replica.Host, replica.GetPort("raft"))
	}
	return p.memberAddresses
}

func (p *Protocol) getAddress(id uint64) string {
	return p.getAddresses()[id]
}

// Start starts the Raft protocol
func (p *Protocol) Start(c cluster.Cluster, registry *protocol.Registry) error {
	member, ok := c.Member()
	if !ok {
		return errors.NewInternal("local member not configured")
	}

	address := fmt.Sprintf("%s:%d", member.Host, member.GetPort("raft"))

	replicas := make([]*cluster.Replica, 0, len(c.Replicas()))
	for _, replica := range c.Replicas() {
		replicas = append(replicas, replica)
	}
	sort.Slice(replicas, func(i, j int) bool {
		return replicas[i].ID < replicas[j].ID
	})

	p.mu.Lock()
	p.replicas = replicas
	p.mu.Unlock()

	memberIDs := p.getMemberIDs()
	memberAddresses := p.getAddresses()
	nodeID := p.getNodeID(string(member.ID))

	// Create a listener to wait for a leader to be elected
	eventCh := make(chan RaftEvent)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.watch(ctx, eventCh)

	nodeConfig := raftconfig.NodeHostConfig{
		WALDir:              dataDir,
		NodeHostDir:         dataDir,
		RTTMillisecond:      rttMillisecond,
		RaftAddress:         address,
		RaftEventListener:   p.listener,
		SystemEventListener: p.listener,
	}

	node, err := dragonboat.NewNodeHost(nodeConfig)
	if err != nil {
		return err
	}

	fsmFactory := func(clusterID, nodeID uint64) statemachine.IStateMachine {
		streams := newStreamManager()
		fsm := newStateMachine(c, protocol.PartitionID(clusterID), registry, streams)
		client := newPartition(clusterID, nodeID, node, memberIDs, streams)
		p.mu.Lock()
		p.clients[protocol.PartitionID(clusterID)] = client
		p.mu.Unlock()
		return fsm
	}

	for _, partition := range c.Partitions() {
		config := raftconfig.Config{
			NodeID:             nodeID,
			ClusterID:          uint64(partition.ID()),
			ElectionRTT:        10,
			HeartbeatRTT:       1,
			CheckQuorum:        true,
			SnapshotEntries:    p.config.GetSnapshotThresholdOrDefault(),
			CompactionOverhead: p.config.GetSnapshotThresholdOrDefault() / 10,
		}

		server := newServer(uint64(partition.ID()), memberAddresses, node, config, fsmFactory)
		if err := server.Start(); err != nil {
			return err
		}
		p.mu.Lock()
		p.servers[protocol.PartitionID(partition.ID())] = server
		p.mu.Unlock()
	}

	startedCh := make(chan struct{})
	go func() {
		startedPartitions := make(map[uint64]bool)
		started := false
		for event := range eventCh {
			if leader, ok := event.Event.(*RaftEvent_LeaderUpdated); ok &&
				leader.LeaderUpdated.Term > 0 && leader.LeaderUpdated.Leader != "" {
				startedPartitions[leader.LeaderUpdated.Partition] = true
				if !started && len(startedPartitions) == len(p.servers) {
					close(startedCh)
					started = true
				}
			}
		}
	}()
	<-startedCh
	return nil
}

// Partition returns the given partition client
func (p *Protocol) Partition(partitionID protocol.PartitionID) protocol.Partition {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clients[partitionID]
}

// Partitions returns all partition clients
func (p *Protocol) Partitions() []protocol.Partition {
	p.mu.RLock()
	defer p.mu.RUnlock()
	partitions := make([]protocol.Partition, len(p.clients))
	for i := 0; i < len(p.clients); i++ {
		partitions[i] = p.clients[protocol.PartitionID(i+1)]
	}
	return partitions
}

// Stop stops the Raft protocol
func (p *Protocol) Stop() error {
	var returnErr error
	for _, server := range p.servers {
		if err := server.Stop(); err != nil {
			returnErr = err
		}
	}
	return returnErr
}
