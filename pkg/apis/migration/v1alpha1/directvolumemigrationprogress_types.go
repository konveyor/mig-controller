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

package v1alpha1

import (
	"context"

	"github.com/google/uuid"
	liberr "github.com/konveyor/controller/pkg/error"
	kapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DirectVolumeMigrationProgressSpec defines the desired state of DirectVolumeMigrationProgress
type DirectVolumeMigrationProgressSpec struct {
	ClusterRef *kapi.ObjectReference `json:"clusterRef,omitempty"`
	PodRef     *kapi.ObjectReference `json:"podRef,omitempty"`
}

// DirectVolumeMigrationProgressStatus defines the observed state of DirectVolumeMigrationProgress
type DirectVolumeMigrationProgressStatus struct {
	Conditions                  `json:",omitempty"`
	PodPhase                    kapi.PodPhase    `json:"phase,omitempty"`
	ExitCode                    *int32           `json:"exitCode,omitempty"`
	ContainerElapsedTime        *metav1.Duration `json:"containerElapsedTime,omitempty"`
	LogMessage                  string           `json:"logMessage,omitempty"`
	ObservedDigest              string           `json:"observedDigest,omitempty"`
	LastObservedProgressPercent string           `json:"lastObservedProgressPercent,omitempty"`
	LastObservedTransferRate    string           `json:"lastObservedTransferRate,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DirectVolumeMigrationProgress is the Schema for the directvolumemigrationprogresses API
// +kubebuilder:resource:path=directvolumemigrationprogresses,shortName=dvmp
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=".spec.clusterRef.name"
// +kubebuilder:printcolumn:name="Pod Name",type=string,JSONPath=".spec.podRef.name"
// +kubebuilder:printcolumn:name="Pod Namespace",type=string,JSONPath=".spec.podRef.namespace"
// +kubebuilder:printcolumn:name="Progress Percent",type=string,JSONPath=".status.lastObservedProgressPercent"
// +kubebuilder:printcolumn:name="Transfer Rate",type=string,JSONPath=".status.lastObservedTransferRate"
// +kubebuilder:printcolumn:name="age",type=date,JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
type DirectVolumeMigrationProgress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DirectVolumeMigrationProgressSpec   `json:"spec,omitempty"`
	Status DirectVolumeMigrationProgressStatus `json:"status,omitempty"`
}

func (d *DirectVolumeMigrationProgress) MarkReconciled() {
	u, _ := uuid.NewUUID()
	if d.Annotations == nil {
		d.Annotations = map[string]string{}
	}
	d.Annotations[TouchAnnotation] = u.String()
	d.Status.ObservedDigest = digest(d.Spec)
}

// Get the DirectVolumeMigration that owns this DirectVolumeMigrationProgress. If not owned, return nil.
func (r *DirectVolumeMigrationProgress) GetOwner(client k8sclient.Client) (*DirectVolumeMigration, error) {
	owner := &DirectVolumeMigration{}
	ownerRefs := r.GetOwnerReferences()
	if len(ownerRefs) > 0 {
		ownerRef := types.NamespacedName{Name: ownerRefs[0].Name, Namespace: r.Namespace}
		err := client.Get(context.TODO(), ownerRef, owner)
		if err != nil {
			return nil, liberr.Wrap(err)
		}
	}
	return owner, nil
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DirectVolumeMigrationProgressList contains a list of DirectVolumeMigrationProgress
type DirectVolumeMigrationProgressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DirectVolumeMigrationProgress `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DirectVolumeMigrationProgress{}, &DirectVolumeMigrationProgressList{})
}
