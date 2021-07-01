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
	"github.com/atomix/atomix-go-framework/pkg/atomix/cluster"
	protocol "github.com/atomix/atomix-go-framework/pkg/atomix/storage/protocol/rsm"
	"github.com/atomix/atomix-go-framework/pkg/atomix/stream"
	"github.com/atomix/atomix-raft-storage/pkg/storage/config"
	"github.com/gogo/protobuf/proto"
	"github.com/lni/dragonboat/v3/statemachine"
	"io"
	"sync"
	"time"
)

// newStateMachine returns a new primitive state machine
func newStateMachine(cluster cluster.Cluster, partitionID protocol.PartitionID, config config.ProtocolConfig, registry *protocol.Registry, streams *streamManager) *StateMachine {
	sessionTimeout := time.Minute
	if config.SessionTimeout != nil {
		sessionTimeout = *config.SessionTimeout
	}
	return &StateMachine{
		partition: partitionID,
		state:     protocol.NewManager(cluster, registry, sessionTimeout),
		streams:   streams,
	}
}

// StateMachine is a Raft state machine
type StateMachine struct {
	partition protocol.PartitionID
	state     *protocol.Manager
	streams   *streamManager
	mu        sync.Mutex
}

// Update updates the state machine state
func (s *StateMachine) Update(bytes []byte) (statemachine.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tsEntry := &Entry{}
	if err := proto.Unmarshal(bytes, tsEntry); err != nil {
		return statemachine.Result{}, err
	}

	stream := s.streams.getStream(tsEntry.StreamID)
	s.state.Command(tsEntry.Value, stream)
	return statemachine.Result{}, nil
}

// Lookup queries the state machine state
func (s *StateMachine) Lookup(value interface{}) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := value.(queryContext)
	s.state.Query(query.value, query.stream)
	return nil, nil
}

// SaveSnapshot saves a snapshot of the state machine state
func (s *StateMachine) SaveSnapshot(writer io.Writer, files statemachine.ISnapshotFileCollection, done <-chan struct{}) error {
	return s.state.Snapshot(writer)
}

// RecoverFromSnapshot recovers the state machine state from a snapshot
func (s *StateMachine) RecoverFromSnapshot(reader io.Reader, files []statemachine.SnapshotFile, done <-chan struct{}) error {
	return s.state.Install(reader)
}

// Close closes the state machine
func (s *StateMachine) Close() error {
	return nil
}

type queryContext struct {
	value  []byte
	stream stream.WriteStream
}
