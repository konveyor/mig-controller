package gvk

import (
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migapi "github.com/konveyor/mig-controller/pkg/apis/migration/v1alpha1"
)

var crdGVR = schema.GroupVersionResource{
	Group:    "apiextensions.k8s.io",
	Version:  "v1beta1", // Should become v1 after 1.17, needs downscaling
	Resource: "customresourcedefinitions",
}

// Compare is a store for discovery and dynamic clients to do GVK compare
type Compare struct {
	Plan         *migapi.MigPlan
	SrcDiscovery discovery.DiscoveryInterface
	DstDiscovery discovery.DiscoveryInterface
	SrcClient    dynamic.Interface
}

// Compare GVKs on both clusters, find unsupported GVRs
// and check each plan source namespace for existence of unsupported GVRs
func (r *Compare) Compare() (map[string][]schema.GroupVersionResource, error) {
	srcResourceList, err := collectResources(r.SrcDiscovery)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to fetch server resources for a srcCluster")
	}

	dstResourceList, err := collectResources(r.DstDiscovery)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to fetch server resources for a dstCluster")
	}

	err = r.excludeCRDs(srcResourceList)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to exclude CRs from the unsupported resources")
	}

	resourcesDiff := compareResources(srcResourceList, dstResourceList)
	unsupportedGVRs, err := r.unsupportedResources(resourcesDiff)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to get unsupported resources for srcCluster")
	}

	return r.collectUnsupportedMapping(unsupportedGVRs)
}

// NewSourceDiscovery initializes source discovery client for a source cluster
func (r *Compare) NewSourceDiscovery(c client.Client) error {
	srcCluster, err := r.Plan.GetSourceCluster(c)
	if err != nil {
		return errors.Wrap(err, "Error reading srcMigCluster")
	}

	discovery, err := r.getDiscovery(c, srcCluster)
	if err != nil {
		return errors.Wrap(err, "Can't compile discovery client for srcCluster")
	}

	r.SrcDiscovery = discovery

	return nil
}

// NewDestinationDiscovery initializes destination discovery client forom a destination cluster
func (r *Compare) NewDestinationDiscovery(c client.Client) error {
	dstCluster, err := r.Plan.GetDestinationCluster(c)
	if err != nil {
		return errors.Wrap(err, "Error reading dstMigCluster")
	}

	discovery, err := r.getDiscovery(c, dstCluster)
	if err != nil {
		return errors.Wrap(err, "Can't compile discovery client for dstCluster")
	}

	r.DstDiscovery = discovery

	return nil
}

// NewSourceClient initializes source discovery client for a source cluster
func (r *Compare) NewSourceClient(c client.Client) error {
	srcCluster, err := r.Plan.GetSourceCluster(c)
	if err != nil {
		return errors.Wrap(err, "Error reading srcMigCluster")
	}

	client, err := r.getClient(c, srcCluster)
	if err != nil {
		return errors.Wrap(err, "Can't compile dynamic client for srcCluster")
	}

	r.SrcClient = client

	return nil
}

func (r *Compare) getDiscovery(c client.Client, cluster *migapi.MigCluster) (*discovery.DiscoveryClient, error) {
	config, err := cluster.BuildRestConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "Can't get REST config from a cluster")
	}

	return discovery.NewDiscoveryClientForConfig(config)
}

func (r *Compare) getClient(c client.Client, cluster *migapi.MigCluster) (dynamic.Interface, error) {
	config, err := cluster.BuildRestConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "Can't get REST config from a cluster")
	}

	return dynamic.NewForConfig(config)
}

func (r *Compare) collectUnsupportedMapping(unsupportedResources []schema.GroupVersionResource) (map[string][]schema.GroupVersionResource, error) {
	unsupportedNamespaces := map[string][]schema.GroupVersionResource{}
	for _, gvr := range unsupportedResources {
		namespaceOccurence, err := r.occurIn(gvr)
		if err != nil {
			return nil, errors.Wrapf(err, "Unable to collect namespace occurences for GVR: %s", gvr)
		}

		for _, namespace := range namespaceOccurence {
			if inNamespaces(namespace, r.Plan.GetSourceNamespaces()) {
				_, exist := unsupportedNamespaces[namespace]
				if exist {
					unsupportedNamespaces[namespace] = append(unsupportedNamespaces[namespace], gvr)
				} else {
					unsupportedNamespaces[namespace] = []schema.GroupVersionResource{gvr}
				}
			}
		}
	}

	return unsupportedNamespaces, nil
}

func (r *Compare) occurIn(gvr schema.GroupVersionResource) ([]string, error) {
	namespacesOccured := []string{}
	options := metav1.ListOptions{}
	resourceList, err := r.SrcClient.Resource(gvr).List(options)
	if err != nil {
		return nil, errors.Wrapf(err, "Error while listing: %s", gvr)
	}

	for _, res := range resourceList.Items {
		if !inNamespaces(res.GetNamespace(), namespacesOccured) {
			namespacesOccured = append(namespacesOccured, res.GetNamespace())
		}
	}

	return namespacesOccured, nil
}

func collectResources(discovery discovery.DiscoveryInterface) ([]*metav1.APIResourceList, error) {
	resources, err := discovery.ServerResources()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to get a list of resources for a GroupVersion on srcCluster")
	}

	for _, res := range resources {
		res.APIResources = namespaced(res.APIResources)
		res.APIResources = excludeSubresources(res.APIResources)
		// Some resources appear not to have permissions to list, need to exclude those.
		res.APIResources = listAllowed(res.APIResources)
	}

	return resources, nil
}

func (r *Compare) unsupportedResources(resourceDiff []*metav1.APIResourceList) ([]schema.GroupVersionResource, error) {
	unsupportedGVRs := []schema.GroupVersionResource{}
	for _, resourceList := range resourceDiff {
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing GroupVersion %s", resourceList.GroupVersion)
		}

		for _, resource := range resourceList.APIResources {
			gvr := gv.WithResource(resource.Name)
			unsupportedGVRs = append(unsupportedGVRs, gvr)
		}
	}

	return unsupportedGVRs, nil
}

func (r *Compare) excludeCRDs(resources []*metav1.APIResourceList) error {
	options := metav1.ListOptions{}
	crdList, err := r.SrcClient.Resource(crdGVR).List(options)
	if err != nil {
		return errors.Wrap(err, "Error while listing CRDs")
	}

	crdGroups := []string{}
	groupPath := []string{"spec", "group"}
	for _, crd := range crdList.Items {
		group, found, err := unstructured.NestedString(crd.Object, groupPath...)
		if !found {
			return errors.Wrap(err, "Error while extracting CRD group: does not exist")
		}
		if err != nil {
			return errors.Wrap(err, "Error while extracting CRD group")
		}
		crdGroups = append(crdGroups, group)
	}

	updatedLists := []*metav1.APIResourceList{}
	for _, resourceList := range resources {
		if !isCRDGroup(resourceList.GroupVersion, crdGroups) {
			updatedLists = append(updatedLists, resourceList)
		}
	}

	resources = updatedLists

	return nil
}

func excludeSubresources(resources []metav1.APIResource) []metav1.APIResource {
	filteredList := []metav1.APIResource{}
	for _, res := range resources {
		if !strings.Contains(res.Name, "/") {
			filteredList = append(filteredList, res)
		}
	}

	return filteredList
}

func namespaced(resources []metav1.APIResource) []metav1.APIResource {
	filteredList := []metav1.APIResource{}
	for _, res := range resources {
		if res.Namespaced {
			filteredList = append(filteredList, res)
		}
	}

	return filteredList
}

func listAllowed(resources []metav1.APIResource) []metav1.APIResource {
	filteredList := []metav1.APIResource{}
	for _, res := range resources {
		for _, verb := range res.Verbs {
			if verb == "list" {
				filteredList = append(filteredList, res)
				break
			}
		}
	}

	return filteredList
}

func resourceExist(resource metav1.APIResource, resources []metav1.APIResource) bool {
	for _, resourceItem := range resources {
		if resource.Name == resourceItem.Name {
			return true
		}
	}

	return false
}

func compareResources(src []*metav1.APIResourceList, dst []*metav1.APIResourceList) []*metav1.APIResourceList {
	missingResources := []*metav1.APIResourceList{}
	for _, srcList := range src {
		missing := []metav1.APIResource{}
		for _, resource := range srcList.APIResources {
			if !resourceExist(resource, fingResourceList(srcList.GroupVersion, dst)) {
				missing = append(missing, resource)
			}
		}

		if len(missing) > 0 {
			missingList := &metav1.APIResourceList{
				GroupVersion: srcList.GroupVersion,
				APIResources: missing,
			}
			missingResources = append(missingResources, missingList)
		}
	}

	return missingResources
}

func fingResourceList(groupVersion string, list []*metav1.APIResourceList) []metav1.APIResource {
	for _, l := range list {
		if l.GroupVersion == groupVersion {
			return l.APIResources
		}
	}

	return nil
}

func inNamespaces(item string, namespaces []string) bool {
	for _, ns := range namespaces {
		if item == ns {
			return true
		}
	}

	return false
}

func isCRDGroup(group string, crdGroups []string) bool {
	for _, crdGroup := range crdGroups {
		if strings.HasPrefix(group, crdGroup) {
			return true
		}
	}

	return false
}
