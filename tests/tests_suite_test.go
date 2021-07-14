package tests

import (
	"context"
	migapi "github.com/konveyor/mig-controller/pkg/apis/migration/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	// +kubebuilder:scaffold:imports
)

func TestMigmigration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var sourceClient *kubernetes.Clientset
var hostClient client.Client
var hostCfg *rest.Config
var sourceCfg *rest.Config
var err error
var _ = BeforeSuite(func() {
	hostCfg, err = clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv(HOSTCONFIG)))
	if err != nil {
		log.Println(err)
	}

	err = migapi.AddToScheme(scheme.Scheme)
	hostClient, err = client.New(hostCfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Println(err)
	}
	sourceCfg, err = clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv(SOURCECONFIG)))
	if err != nil {
		log.Println(err)
	}
	sourceClient, err = kubernetes.NewForConfig(sourceCfg)
	if err != nil {
		log.Println(err)
	}

	migCluster, secret := NewMigCluster(GetMigSaToken(sourceClient))
	Expect(hostClient.Create(ctx, secret)).Should(Succeed())
	Expect(hostClient.Create(ctx, migCluster)).Should(Succeed())
	Eventually(func() string {
		hostClient.Get(ctx, client.ObjectKey{Name: TestObjectName, Namespace: MigrationNamespace}, migCluster)
		l := len(migCluster.Status.Conditions.List)
		if l > 0 {
			return migCluster.Status.Conditions.List[l-1].Type
		}
		return ""
	}, time.Minute*5, time.Second).Should(Equal("Ready"))

	migStorage, secret := NewMigStorage()
	Expect(hostClient.Create(ctx, secret)).Should(Succeed())
	Expect(hostClient.Create(ctx, migStorage)).Should(Succeed())
	Eventually(func() string {
		hostClient.Get(ctx, client.ObjectKey{Name: TestObjectName, Namespace: MigrationNamespace}, migStorage)
		l := len(migStorage.Status.Conditions.List)
		if l > 0 {
			return migStorage.Status.Conditions.List[l-1].Type
		}
		return ""
	}, time.Minute*5, time.Second).Should(Equal("Ready"))

}, 60)

var _ = AfterSuite(func() {

	ctx := context.TODO()
	err = hostClient.Delete(ctx, &migapi.MigStorage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestObjectName,
			Namespace: MigrationNamespace,
		},
	})
	if err != nil {
		log.Println(err)
	}

	err = hostClient.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestStorageSecret,
			Namespace: ConfigNamespace,
		},
	})
	if err != nil {
		log.Println(err)
	}

	err = hostClient.Delete(ctx, &migapi.MigCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestObjectName,
			Namespace: MigrationNamespace,
		},
	})
	if err != nil {
		log.Println(err)
	}

	err = hostClient.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestClusterSecret,
			Namespace: ConfigNamespace,
		},
	})
	if err != nil {
		log.Println(err)
	}
})
