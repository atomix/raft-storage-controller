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

package v2beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RaftMemberState string

const (
	RaftMemberNotReady RaftMemberState = "NotReady"
	RaftMemberReady    RaftMemberState = "Ready"
)

type RaftMemberRole string

const (
	RaftLeader    RaftMemberRole = "Leader"
	RaftCandidate RaftMemberRole = "Candidate"
	RaftFollower  RaftMemberRole = "Follower"
)

// RaftMemberSpec specifies a RaftMemberSpec configuration
type RaftMemberSpec struct {
	ClusterID   int32  `json:"clusterId,omitempty"`
	PartitionID int32  `json:"partitionId,omitempty"`
	MemberID    int32  `json:"memberId,omitempty"`
	Pod         string `json:"pod,omitempty"`
}

// RaftMemberStatus defines the status of a RaftMember
type RaftMemberStatus struct {
	State             *RaftMemberState `json:"state,omitempty"`
	Role              *RaftMemberRole  `json:"role,omitempty"`
	Leader            *string          `json:"leader,omitempty"`
	Term              *uint64          `json:"term,omitempty"`
	LastUpdated       *metav1.Time     `json:"lastUpdated,omitempty"`
	LastSnapshotIndex *uint64          `json:"lastSnapshotIndex,omitempty"`
	LastSnapshotTime  *metav1.Time     `json:"lastSnapshotTime,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RaftMember is the Schema for the RaftMember API
// +k8s:openapi-gen=true
type RaftMember struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RaftMemberSpec   `json:"spec,omitempty"`
	Status            RaftMemberStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RaftMemberList contains a list of RaftMember
type RaftMemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the RaftMember of items in the list
	Items []RaftMember `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RaftMember{}, &RaftMemberList{})
}
