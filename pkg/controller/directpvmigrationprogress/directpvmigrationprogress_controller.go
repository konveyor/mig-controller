/*
Copyright 2019 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package directpvmigrationprogress

import (
	"context"
	"fmt"
	"path"
	"time"

	liberr "github.com/konveyor/controller/pkg/error"
	"github.com/konveyor/controller/pkg/logging"
	migapi "github.com/konveyor/mig-controller/pkg/apis/migration/v1alpha1"
	migref "github.com/konveyor/mig-controller/pkg/reference"
	kapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logging.WithName("pvmigrationprogress")

const (
	NotFound    = "NotFound"
	NotSet      = "NotSet"
	NotDistinct = "NotDistinct"
	NotReady    = "NotReady"
)

const (
	InvalidClusterRef = "InvalidClusterRef"
	ClusterNotReady   = "ClusterNotReady"
	InvalidPodRef     = "InvalidPodRef"
	InvalidPod        = "InvalidPod"
)

// Add creates a new DirectPVMigrationProgress Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDirectPVMigrationProgress{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("directpvmigrationprogress-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to DirectPVMigrationProgress
	err = c.Watch(&source.Kind{Type: &migapi.DirectPVMigrationProgress{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by DirectPVMigrationProgress - change this for objects you create
	//err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
	//	IsController: true,
	//	OwnerType:    &migrationv1alpha1.DirectPVMigrationProgress{},
	//})
	//if err != nil {
	//	return err
	//}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDirectPVMigrationProgress{}

// ReconcileDirectPVMigrationProgress reconciles a DirectPVMigrationProgress object
type ReconcileDirectPVMigrationProgress struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a DirectPVMigrationProgress object and makes changes based on the state read
// and what is in the DirectPVMigrationProgress.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=migration.openshift.io,resources=directpvmigrationprogresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=migration.openshift.io,resources=directpvmigrationprogresses/status,verbs=get;update;patch
func (r *ReconcileDirectPVMigrationProgress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	var err error
	log.Reset()
	// Fetch the DirectPVMigrationProgress instance
	pvProgress := &migapi.DirectPVMigrationProgress{}
	err = r.Get(context.TODO(), request.NamespacedName, pvProgress)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Report reconcile error.
	defer func() {
		if err == nil || errors.IsConflict(err) {
			return
		}
		pvProgress.Status.SetReconcileFailed(err)
		err := r.Update(context.TODO(), pvProgress)
		if err != nil {
			log.Trace(err)
			return
		}
	}()

	// Begin staging conditions.
	pvProgress.Status.BeginStagingConditions()

	err = r.reportContainerStatus(pvProgress, "rsync-client")
	if err != nil {
		return reconcile.Result{Requeue: true}, liberr.Wrap(err)
	}

	pvProgress.Status.SetReady(!pvProgress.Status.HasCriticalCondition(), "The progress is available")
	if pvProgress.Status.HasCriticalCondition() {
		pvProgress.Status.PodPhase = ""
	}

	pvProgress.Status.EndStagingConditions()

	pvProgress.MarkReconciled()
	err = r.Update(context.TODO(), pvProgress)
	if err != nil {
		log.Trace(err)
		return reconcile.Result{Requeue: true}, nil
	}

	if !pvProgress.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}

	// we will requeue this every 5 seconds
	return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil

	// at this point assume that the container is running
	//pipe, err := r.ProgressPipe(pvProgress)
	//if err != nil {
	//	return reconcile.Result{}, err
	//}

	//progress, err := r.ProgressPipe(pvProgress)
	////err := r.ReportProgress(progress)
	//if err != nil {
	//	return reconcile.Result{}, err
	//}
	//
	//fmt.Println(pipe)
}

func (r *ReconcileDirectPVMigrationProgress) reportContainerStatus(pvProgress *migapi.DirectPVMigrationProgress, containerName string) error {
	podRef := pvProgress.Spec.PodRef
	ref := pvProgress.Spec.ClusterRef

	// NotSet
	if !migref.RefSet(ref) {
		pvProgress.Status.SetCondition(migapi.Condition{
			Type:     InvalidClusterRef,
			Status:   migapi.True,
			Reason:   NotSet,
			Category: migapi.Critical,
			Message:  "The spec.clusterRef must reference name and namespace of a valid `MigCluster",
		})
		return nil
	}

	cluster, err := migapi.GetCluster(r, ref)
	if err != nil {
		return liberr.Wrap(err)
	}

	// NotFound
	if cluster == nil {
		pvProgress.Status.SetCondition(migapi.Condition{
			Type:     InvalidClusterRef,
			Status:   migapi.True,
			Reason:   NotFound,
			Category: migapi.Critical,
			Message: fmt.Sprintf("The spec.clusterRef must reference a valid `MigCluster` %s",
				path.Join(ref.Namespace, ref.Name)),
		})
		return nil
	}

	// Not ready
	if !cluster.Status.IsReady() {
		pvProgress.Status.SetCondition(migapi.Condition{
			Type:     ClusterNotReady,
			Status:   migapi.True,
			Reason:   NotReady,
			Category: migapi.Critical,
			Message: fmt.Sprintf("The `MigCluster` spec.ClusterRef %s is not ready",
				path.Join(ref.Namespace, ref.Name)),
		})
	}

	pod, err := r.Pod(cluster, podRef)
	switch {
	case errors.IsNotFound(err):
		// handle not found and return
		pvProgress.Status.SetCondition(migapi.Condition{
			Type:     InvalidPod,
			Status:   migapi.True,
			Reason:   NotFound,
			Category: migapi.Critical,
			Message: fmt.Sprintf("The spec.podRef %s must reference a valid `Pod` ",
				path.Join(podRef.Namespace, podRef.Name)),
		})
		return nil
	case err != nil:
		return liberr.Wrap(err)
	}

	var containerStatus *kapi.ContainerStatus
	for _, c := range pod.Status.ContainerStatuses {
		if c.Name == containerName {
			containerStatus = &c
		}
	}

	if containerStatus == nil {
		pvProgress.Status.SetCondition(migapi.Condition{
			Type:     InvalidPod,
			Status:   migapi.True,
			Reason:   NotFound,
			Category: migapi.Critical,
			Message: fmt.Sprintf("The spec.podRef %s must reference a `Pod` with container name %s",
				path.Join(podRef.Namespace, podRef.Name), containerName),
		})
		return nil
	}

	switch {
	case containerStatus.Ready:
		// report pod running and return
		pvProgress.Status.PodPhase = kapi.PodRunning
	case !containerStatus.Ready && containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode != 0:
		// pod has a failure, report last failure reason
		pvProgress.Status.PodPhase = kapi.PodFailed
		// TODO: report failure
	case !containerStatus.Ready && containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 0:
		// succeeded dont ever requeue
		pvProgress.Status.PodPhase = kapi.PodSucceeded
	case pod.Status.Phase == kapi.PodSucceeded:
		// Its possible for the succeeded pod to not have containerStatuses at all
		pvProgress.Status.PodPhase = kapi.PodSucceeded
	}

	return nil
}

func (r *ReconcileDirectPVMigrationProgress) Pod(cluster *migapi.MigCluster, podReference *kapi.ObjectReference) (*kapi.Pod, error) {
	cli, err := cluster.GetClient(r)
	if err != nil {
		return nil, liberr.Wrap(err)
	}
	pod := &kapi.Pod{}
	err = cli.Get(context.TODO(), types.NamespacedName{
		Namespace: podReference.Namespace,
		Name:      podReference.Name,
	}, pod)
	if err != nil {
		return nil, err
	}
	return pod, nil
}
