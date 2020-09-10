// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	cert "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	observatoriumv1alpha1 "github.com/observatorium/deployments/operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	placementv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

func init() {
	os.Setenv("TEMPLATES_PATH", "../../../manifests/")
}

func TestLabelsForMultiClusterMonitoring(t *testing.T) {
	lab := labelsForMultiClusterMonitoring("test")

	value, _ := lab["observability.open-cluster-management.io/name"]
	if value != "test" {
		t.Errorf("value (%v) is not the expected (test)", value)
	}
}

func createObservatoriumApiService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-observatorium-observatorium-api",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "api",
				"app.kubernetes.io/instance":  name + "-observatorium",
			},
		},
		Spec: corev1.ServiceSpec{},
	}
}

func createReadyDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-fake-ready-deployment",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name + "-fake-deployment",
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     1,
			AvailableReplicas: 1,
		},
	}
}

func createFailedDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-fake-failed-deployment",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name + "-fake-deployment",
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 0,
		},
	}
}

func createClusterVersion() *configv1.ClusterVersion {
	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID("xxx-xxxxxx-xxxx"),
		},
	}
}

func TestMultiClusterMonitoringCRUpdate(t *testing.T) {
	var (
		name      = "monitoring"
		namespace = mcoconfig.GetDefaultNamespace()
	)
	logf.SetLogger(logf.ZapLogger(true))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	//provide a non-existence path to bypass the rendering
	//cannot convert unstructured.Unstructured into v1.Service in fake client
	os.Setenv("TEMPLATES_PATH", path.Join(wd, "../../../tests/manifests"))

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta1.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)
	placementv1.AddToScheme(s)
	cert.AddToScheme(s)

	svc := createObservatoriumApiService(name, namespace)
	grafanaCert := newTestCert(GetGrafanaCerts(), namespace)
	serverCert := newTestCert(GetServerCerts(), namespace)

	objs := []runtime.Object{mco, svc, grafanaCert, serverCert}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	ocpClient := fakeconfigclient.NewSimpleClientset([]runtime.Object{createClusterVersion()}...)
	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &ReconcileMultiClusterObservability{client: cl, scheme: s, ocpClient: ocpClient}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	updatedMCO := &mcov1beta1.MultiClusterObservability{}
	err = cl.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	if updatedMCO.Status.Conditions[0].Failed.Message != "No deployment found." {
		t.Fatalf("Failed to get correct MCO status, expect failed with no deployment")
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)

	// A MultiClusterObservability object with metadata and spec.
	mco = &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       mcov1beta1.MultiClusterObservabilitySpec{},
	}
	readyDeployment := createReadyDeployment(name, namespace)
	err = cl.Update(context.TODO(), mco)
	err = cl.Create(context.TODO(), readyDeployment)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)
	updatedMCO = &mcov1beta1.MultiClusterObservability{}
	err = r.client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	if updatedMCO.Status.Conditions[0].Ready.Type != "Ready" {
		t.Fatalf("Failed to get correct MCO status, expect Ready")
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)

	failedDeployment := createFailedDeployment(name, namespace)
	err = cl.Create(context.TODO(), failedDeployment)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)
	updatedMCO = &mcov1beta1.MultiClusterObservability{}
	err = r.client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	if updatedMCO.Status.Conditions[0].Failed.Message != "Deployment failed for monitoring-fake-failed-deployment" {
		t.Fatalf("Failed to get correct MCO status, expect failed with failed deployment")
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)

	//Test finalizer
	mco.ObjectMeta.DeletionTimestamp = &v1.Time{time.Now()}
	mco.ObjectMeta.Finalizers = []string{certFinalizer, "test-finalizerr"}
	err = cl.Update(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile for finalizer: (%v)", err)
	}
}
