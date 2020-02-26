package container

import (
	"context"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

// A collection of k8s Service resources.
type Service struct {
	// Base
	BaseCollection
}

func (r *Service) AddWatch(dsController controller.Controller) error {
	err := dsController.Watch(
		&source.Kind{
			Type: &v1.Service{},
		},
		&handler.EnqueueRequestForObject{},
		r)
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

func (r *Service) Reconcile() error {
	mark := time.Now()
	sr := SimpleReconciler{
		Db: r.ds.Container.Db,
	}
	err := sr.Reconcile(r)
	if err != nil {
		Log.Trace(err)
		return err
	}
	r.hasReconciled = true
	Log.Info(
		"Service (collection) reconciled.",
		"ns",
		r.ds.Cluster.Namespace,
		"name",
		r.ds.Cluster.Name,
		"duration",
		time.Since(mark))

	return nil
}

func (r *Service) GetDiscovered() ([]model.Model, error) {
	models := []model.Model{}
	onCluster := v1.ServiceList{}
	err := r.ds.Client.List(context.TODO(), nil, &onCluster)
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	for _, discovered := range onCluster.Items {
		service := &model.Service{
			Base: model.Base{
				Cluster: r.ds.Cluster.PK,
			},
		}
		service.With(&discovered)
		models = append(models, service)
	}

	return models, nil
}

func (r *Service) GetStored() ([]model.Model, error) {
	models := []model.Model{}
	list, err := model.Service{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}.List(
		r.ds.Container.Db,
		model.ListOptions{})
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	for _, service := range list {
		models = append(models, service)
	}

	return models, nil
}

//
// Predicate methods.
//

func (r *Service) Create(e event.CreateEvent) bool {
	Log.Reset()
	object, cast := e.Object.(*v1.Service)
	if !cast {
		return false
	}
	service := model.Service{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}
	service.With(object)
	r.ds.Create(&service)

	return false
}

func (r *Service) Update(e event.UpdateEvent) bool {
	Log.Reset()
	object, cast := e.ObjectNew.(*v1.Service)
	if !cast {
		return false
	}
	service := model.Service{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}
	service.With(object)
	r.ds.Update(&service)

	return false
}

func (r *Service) Delete(e event.DeleteEvent) bool {
	Log.Reset()
	object, cast := e.Object.(*v1.Service)
	if !cast {
		return false
	}
	service := model.Service{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}
	service.With(object)
	r.ds.Delete(&service)

	return false
}

func (r *Service) Generic(e event.GenericEvent) bool {
	return false
}
