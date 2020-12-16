package directvolumemigration

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Task) areSourcePVCsUnattached() error {
	// This function provides state checking on source PVCs, make sure app is
	// quiesced
	return nil
}

func (t *Task) createDestinationPVCs() error {
	// Get client for destination
	destClient, err := t.getDestinationClient()
	if err != nil {
		return err
	}

	// Get client for source
	srcClient, err := t.getSourceClient()
	if err != nil {
		return err
	}

	for _, pvc := range t.Owner.Spec.PersistentVolumeClaims {
		// Get pvc definition from source cluster

		srcPVC := corev1.PersistentVolumeClaim{}
		key := types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}
		err = srcClient.Get(context.TODO(), key, &srcPVC)
		if err != nil {
			return err
		}

		newSpec := srcPVC.Spec
		srcPVC.Labels = t.buildDVMLabels()
		newSpec.StorageClassName = &pvc.TargetStorageClass
		newSpec.AccessModes = pvc.TargetAccessModes
		newSpec.VolumeName = ""

		//Add src labels and rollback labels
		pvcLabels := srcPVC.Labels
		if pvcLabels == nil {
			pvcLabels = make(map[string]string)
		}

		if t.MigrationUID != "" && t.PlanResources.MigPlan != nil {
			pvcLabels[MigratedByMigrationLabel] = t.MigrationUID
			pvcLabels[MigratedByPlanLabel] = string(t.PlanResources.MigPlan.UID)
		}

		// Create pvc on destination with same metadata + spec
		destPVC := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
				Labels:    pvcLabels,
			},
			Spec: newSpec,
		}
		err = destClient.Create(context.TODO(), &destPVC)
		if k8serror.IsAlreadyExists(err) {
			t.Log.Info("PVC already exists on destination", "name", pvc.Name)
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (t *Task) getDestinationPVCs() (bool, error) {
	// Ensure PVCs are bound and not in pending state
	// Get client for destination
	destClient, err := t.getDestinationClient()
	if err != nil {
		return false, err
	}

	pvcMap := t.getPVCNamespaceMap()
	selector := labels.SelectorFromSet(t.buildDVMLabels())
	for ns, _ := range pvcMap {
		pvcList := corev1.PersistentVolumeClaimList{}
		err = destClient.List(
			context.TODO(),
			&k8sclient.ListOptions{
				Namespace:     ns,
				LabelSelector: selector,
			},
			&pvcList)
		if err != nil {
			return false, err
		}
		for _, pvc := range pvcList.Items {
			if pvc.Status.Phase == corev1.ClaimLost {
				t.Log.Info("The PVC in lost state", pvc.Name, pvc.Status.Phase)
			} else if pvc.Status.Phase != corev1.ClaimBound {
				return false, nil
			}
		}
	}
	return true, nil
}
