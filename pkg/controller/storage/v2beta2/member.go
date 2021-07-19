// Copyright 2021-present Open Networking Foundation.
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

package v2beta2

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sync"
	"time"

	storagev2beta2 "github.com/atomix/atomix-raft-storage/pkg/apis/storage/v2beta2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func addRaftMemberController(mgr manager.Manager) error {
	options := controller.Options{
		Reconciler: &RaftMemberReconciler{
			client:  mgr.GetClient(),
			scheme:  mgr.GetScheme(),
			events:  mgr.GetEventRecorderFor("atomix-raft-storage"),
			streams: make(map[string]func()),
		},
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Millisecond*10, time.Second*5),
	}

	// Create a new controller
	controller, err := controller.New(mgr.GetScheme().Name(), mgr, options)
	if err != nil {
		return err
	}

	// Watch for changes to the storage resource and enqueue RaftMembers that reference it
	err = controller.Watch(&source.Kind{Type: &storagev2beta2.RaftMember{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pod
	err = controller.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &storagev2beta2.RaftMember{},
		IsController: false,
	})
	if err != nil {
		return err
	}
	return nil
}

// RaftMemberReconciler reconciles a RaftMember object
type RaftMemberReconciler struct {
	client  client.Client
	scheme  *runtime.Scheme
	events  record.EventRecorder
	streams map[string]func()
	mu      sync.Mutex
}

// Reconcile reads that state of the cluster for a Cluster object and makes changes based on the state read
// and what is in the Cluster.Spec
func (r *RaftMemberReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Info("Reconcile RaftMember")
	group := &storagev2beta2.RaftMember{}
	err := r.client.Get(context.TODO(), request.NamespacedName, group)
	if err != nil {
		log.Error(err, "Reconcile RaftMember")
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	ok, err := r.reconcileMembers(cluster, group)
	if err != nil {
		log.Error(err, "Reconcile RaftMember")
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

var _ reconcile.Reconciler = (*RaftMemberReconciler)(nil)