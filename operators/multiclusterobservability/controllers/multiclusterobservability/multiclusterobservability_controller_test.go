// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	observatoriumv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"

	mchv1 "github.com/open-cluster-management/multiclusterhub-operator/pkg/apis/operator/v1"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	mcoshared "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func init() {
	os.Setenv("UNIT_TEST", "true")
}

func newTestCert(name string, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("test-ca-crt"),
			"tls.crt": []byte("test-tls-crt"),
			"tls.key": []byte("test-tls-key"),
		},
	}
}

var testImagemanifestsMap = map[string]string{
	"endpoint_monitoring_operator": "test.io/endpoint-monitoring:test",
	"grafana":                      "test.io/grafana:test",
	"grafana_dashboard_loader":     "test.io/grafana-dashboard-loader:test",
	"management_ingress":           "test.io/management-ingress:test",
	"observatorium":                "test.io/observatorium:test",
	"observatorium_operator":       "test.io/observatorium-operator:test",
	"prometheus_alertmanager":      "test.io/prometheus-alertmanager:test",
	"prometheus-config-reloader":   "test.io/configmap-reloader:test",
	"rbac_query_proxy":             "test.io/rbac-query-proxy:test",
	"thanos":                       "test.io/thanos:test",
	"thanos_receive_controller":    "test.io/thanos_receive_controller:test",
}

func newTestImageManifestsConfigMap(namespace, version string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ImageManifestConfigMapNamePrefix + version,
			Namespace: namespace,
			Labels: map[string]string{
				config.OCMManifestConfigMapTypeLabelKey:    config.OCMManifestConfigMapTypeLabelValue,
				config.OCMManifestConfigMapVersionLabelKey: version,
			},
		},
		Data: testImagemanifestsMap,
	}
}

func newMCHInstanceWithVersion(namespace, version string) *mchv1.MultiClusterHub {
	return &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: namespace,
		},
		Spec: mchv1.MultiClusterHubSpec{},
		Status: mchv1.MultiClusterHubStatus{
			CurrentVersion: version,
			DesiredVersion: version,
		},
	}
}

func TestLabelsForMultiClusterMonitoring(t *testing.T) {
	lab := labelsForMultiClusterMonitoring("test")

	value, _ := lab["observability.open-cluster-management.io/name"]
	if value != "test" {
		t.Errorf("value (%v) is not the expected (test)", value)
	}
}

func createObservatoriumAPIService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-observatorium-api",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "api",
				"app.kubernetes.io/instance":  name,
			},
		},
		Spec: corev1.ServiceSpec{},
	}
}

func newClusterManagementAddon() *addonv1alpha1.ClusterManagementAddOn {
	return &addonv1alpha1.ClusterManagementAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterManagementAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "ObservabilityController",
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: "ObservabilityController",
				Description: "ObservabilityController Description",
			},
			AddOnConfiguration: addonv1alpha1.ConfigCoordinates{
				CRDName: "observabilityaddons.observability.open-cluster-management.io",
			},
		},
	}
}

func createReadyStatefulSet(name, namespace, statefulSetName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
			Labels: map[string]string{
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 1,
			Replicas:      1,
		},
	}
}

func createFailedStatefulSet(name, namespace, statefulSetName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
			Labels: map[string]string{
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 0,
		},
	}
}

func createReadyDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name,
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     1,
			AvailableReplicas: 1,
			Replicas:          1,
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
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name,
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

func createMultiClusterHubCRD() *apiextensionsv1beta1.CustomResourceDefinition {
	return &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: config.MCHCrdName},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Scope:      apiextensionsv1beta1.NamespaceScoped,
			Conversion: &apiextensionsv1beta1.CustomResourceConversion{Strategy: apiextensionsv1beta1.NoneConverter},
			Group:      "operator.open-cluster-management.io",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:       "MultiClusterHub",
				ListKind:   "MultiClusterHubList",
				Plural:     "multiclusterhubs",
				ShortNames: []string{"mch"},
				Singular:   "multiclusterhub",
			},
			Version: "v1",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Storage: true, Served: true},
			},
		},
	}
}

func TestMultiClusterMonitoringCRUpdate(t *testing.T) {
	var (
		name      = "monitoring"
		namespace = config.GetDefaultNamespace()
	)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	testManifestsPath := path.Join(wd, "../../tests/manifests")
	os.Setenv("TEMPLATES_PATH", testManifestsPath)

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				config.AnnotationKeyImageTagSuffix: "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            "gp2",
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: false,
			},
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)
	clusterv1.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)
	migrationv1alpha1.SchemeBuilder.AddToScheme(s)

	svc := createObservatoriumAPIService(name, namespace)
	serverCACerts := newTestCert(config.ServerCACerts, namespace)
	clientCACerts := newTestCert(config.ClientCACerts, namespace)
	grafanaCert := newTestCert(config.GrafanaCerts, namespace)
	serverCert := newTestCert(config.ServerCerts, namespace)
	// byo case for the alertmanager route
	testAmRouteBYOCaSecret := newTestCert(config.AlertmanagerRouteBYOCAName, namespace)
	testAmRouteBYOCertSecret := newTestCert(config.AlertmanagerRouteBYOCERTName, namespace)
	clustermgmtAddon := newClusterManagementAddon()

	objs := []runtime.Object{mco, svc, serverCACerts, clientCACerts, grafanaCert, serverCert,
		testAmRouteBYOCaSecret, testAmRouteBYOCertSecret, clustermgmtAddon}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &MultiClusterObservabilityReconciler{Client: cl, Scheme: s, CRDMap: map[string]bool{}}
	config.SetMonitoringCRName(name)
	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: name,
		},
	}

	// Create empty client
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO := &mcov1beta2.MultiClusterObservability{}
	err = cl.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status := findStatusCondition(updatedMCO.Status.Conditions, "Failed")
	if status == nil || status.Reason != "ObjectStorageSecretNotFound" {
		t.Errorf("Failed to get correct MCO status, expect Failed")
	}

	amRoute := &routev1.Route{}
	err = cl.Get(context.TODO(), types.NamespacedName{
		Name:      config.AlertmanagerRouteName,
		Namespace: namespace,
	}, amRoute)
	if err != nil {
		t.Fatalf("Failed to get alertmanager's route: (%v)", err)
	}
	// check the BYO certificate for alertmanager's route
	if amRoute.Spec.TLS.CACertificate != "test-tls-crt" ||
		amRoute.Spec.TLS.Certificate != "test-tls-crt" ||
		amRoute.Spec.TLS.Key != "test-tls-key" {
		t.Fatalf("incorrect certificate for alertmanager's route")
	}

	err = cl.Create(context.TODO(), createSecret("test", "test", namespace))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	status = findStatusCondition(updatedMCO.Status.Conditions, "Failed")
	if status == nil || status.Reason != "DeploymentNotFound" {
		t.Errorf("Failed to get correct MCO status, expect Failed")
	}
	expectedDeploymentNames := getExpectedDeploymentNames()
	for _, deployName := range expectedDeploymentNames {
		deploy := createReadyDeployment(deployName, namespace)
		err = cl.Create(context.TODO(), deploy)
		if err != nil {
			t.Fatalf("Failed to create deployment %s: %v", deployName, err)
		}
	}

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	status = findStatusCondition(updatedMCO.Status.Conditions, "Failed")
	if status == nil || status.Reason != "StatefulSetNotFound" {
		t.Errorf("Failed to get correct MCO status, expect Failed")
	}

	expectedStatefulSetNames := getExpectedStatefulSetNames()
	for _, statefulName := range expectedStatefulSetNames {
		deploy := createReadyStatefulSet(name, namespace, statefulName)
		err = cl.Create(context.TODO(), deploy)
		if err != nil {
			t.Fatalf("Failed to create stateful set %s: %v", statefulName, err)
		}
	}

	result, err := r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	if result.Requeue {
		_, err = r.Reconcile(context.TODO(), req)
		if err != nil {
			t.Fatalf("reconcile: (%v)", err)
		}
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "Ready")
	if status == nil || status.Reason != "Ready" {
		t.Errorf("Failed to get correct MCO status, expect Ready")
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "MetricsDisabled")
	if status == nil || status.Reason != "MetricsDisabled" {
		t.Errorf("Failed to get correct MCO status, expect MetricsDisabled")
	}

	// test MetricsDisabled status
	err = cl.Delete(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to delete mco: (%v)", err)
	}
	// reconcile to make sure the finalizer of the mco cr is deleted
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	// wait for the stop status update channel is closed
	time.Sleep(1 * time.Second)

	mco.Spec.ObservabilityAddonSpec.EnableMetrics = true
	mco.ObjectMeta.ResourceVersion = ""
	err = cl.Create(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to create mco: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "MetricsDisabled")
	if status != nil {
		t.Errorf("Should have not MetricsDisabled status")
	}

	// test StatefulSetNotReady status
	err = cl.Delete(context.TODO(), createReadyStatefulSet(
		name,
		namespace,
		config.GetOperandNamePrefix()+"alertmanager"))
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}
	failedAlertManager := createFailedStatefulSet(
		name,
		namespace,
		config.GetOperandNamePrefix()+"alertmanager")
	err = cl.Create(context.TODO(), failedAlertManager)
	if err != nil {
		t.Fatalf("Failed to create alertmanager: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "Ready")
	if status == nil || status.Reason != "Ready" {
		t.Errorf("Failed to get correct MCO status, expect Ready")
	}

	// test DeploymentNotReady status
	err = cl.Delete(context.TODO(), createReadyDeployment(config.GetOperandNamePrefix()+"rbac-query-proxy", namespace))
	if err != nil {
		t.Fatalf("Failed to delete rbac-query-proxy: (%v)", err)
	}
	err = cl.Delete(context.TODO(), failedAlertManager)
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}
	err = cl.Create(context.TODO(), createReadyStatefulSet(
		name,
		namespace,
		config.GetOperandNamePrefix()+"alertmanager"))
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}

	failedRbacProxy := createFailedDeployment("rbac-query-proxy", namespace)
	err = cl.Create(context.TODO(), failedRbacProxy)
	if err != nil {
		t.Fatalf("Failed to create rbac-query-proxy: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "Ready")
	if status == nil || status.Reason != "Ready" {
		t.Errorf("Failed to get correct MCO status, expect Ready")
	}

	//Test finalizer
	mco.ObjectMeta.DeletionTimestamp = &v1.Time{Time: time.Now()}
	mco.ObjectMeta.Finalizers = []string{resFinalizer, "test-finalizerr"}
	mco.ObjectMeta.ResourceVersion = updatedMCO.ObjectMeta.ResourceVersion
	err = cl.Update(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile for finalizer: (%v)", err)
	}

}

func TestImageReplaceForMCO(t *testing.T) {
	var (
		name      = "test-monitoring"
		namespace = config.GetDefaultNamespace()
		version   = "2.3.0"
	)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	os.MkdirAll(path.Join(wd, "../../tests"), 0755)
	testManifestsPath := path.Join(wd, "../../tests/manifests")
	manifestsPath := path.Join(wd, "../../manifests")
	os.Setenv("TEMPLATES_PATH", testManifestsPath)
	err = os.Symlink(manifestsPath, testManifestsPath)
	if err != nil {
		t.Fatalf("Failed to create symbollink(%s) to(%s) for the test manifests: (%v)", testManifestsPath, manifestsPath, err)
	}

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            "gp2",
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: false,
			},
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)
	clusterv1.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)
	mchv1.SchemeBuilder.AddToScheme(s)
	migrationv1alpha1.SchemeBuilder.AddToScheme(s)

	observatoriumAPIsvc := createObservatoriumAPIService(name, namespace)
	serverCACerts := newTestCert(config.ServerCACerts, namespace)
	clientCACerts := newTestCert(config.ClientCACerts, namespace)
	grafanaCert := newTestCert(config.GrafanaCerts, namespace)
	serverCert := newTestCert(config.ServerCerts, namespace)
	// create the image manifest configmap
	imageManifestsCM := newTestImageManifestsConfigMap(config.GetMCONamespace(), version)
	// byo case for the alertmanager route
	testAmRouteBYOCaSecret := newTestCert(config.AlertmanagerRouteBYOCAName, namespace)
	testAmRouteBYOCertSecret := newTestCert(config.AlertmanagerRouteBYOCERTName, namespace)
	clustermgmtAddon := newClusterManagementAddon()

	objs := []runtime.Object{mco, observatoriumAPIsvc, serverCACerts, clientCACerts, grafanaCert, serverCert,
		imageManifestsCM, testAmRouteBYOCaSecret, testAmRouteBYOCertSecret, clustermgmtAddon}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &MultiClusterObservabilityReconciler{Client: cl, Scheme: s, CRDMap: map[string]bool{config.MCHCrdName: true}}
	config.SetMonitoringCRName(name)
	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	// Create empty client
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	//wait for update status
	time.Sleep(1 * time.Second)

	expectedDeploymentNames := []string{
		config.GetOperandNamePrefix() + config.Grafana,
		config.GetOperandNamePrefix() + config.ObservatoriumOperator,
		config.GetOperandNamePrefix() + config.RBACQueryProxy,
	}
	for _, deployName := range expectedDeploymentNames {
		deploy := &appsv1.Deployment{}
		err = cl.Get(context.TODO(), types.NamespacedName{
			Name:      deployName,
			Namespace: namespace,
		}, deploy)
		if err != nil {
			t.Fatalf("Failed to get deployment %s: %v", deployName, err)
		}
		for _, container := range deploy.Spec.Template.Spec.Containers {
			imageKey := strings.ReplaceAll(container.Name, "-", "_")
			switch container.Name {
			case "oauth-proxy":
				// TODO: add oauth-proxy image to image manifests
				continue
			case "config-reloader":
				imageKey = "prometheus-config-reloader"
			}
			imageValue, exists := testImagemanifestsMap[imageKey]
			if !exists {
				t.Fatalf("The image key(%s) for the container(%s) doesn't exist in the deployment(%s)", imageKey, container.Name, deployName)
			}
			if imageValue == container.Image {
				t.Fatalf("The image(%s) for the container(%s) in the deployment(%s) should not replace with the one in the image manifests", imageValue, container.Name, deployName)
			}
		}

	}

	expectedStatefulSetNames := []string{
		config.GetOperandNamePrefix() + config.Alertmanager,
	}
	for _, statefulName := range expectedStatefulSetNames {
		sts := &appsv1.StatefulSet{}
		err = cl.Get(context.TODO(), types.NamespacedName{
			Name:      statefulName,
			Namespace: namespace,
		}, sts)
		if err != nil {
			t.Fatalf("Failed to get statefulset %s: %v", statefulName, err)
		}
		for _, container := range sts.Spec.Template.Spec.Containers {
			imageKey := strings.ReplaceAll(container.Name, "-", "_")
			switch container.Name {
			case "oauth-proxy", "alertmanager-proxy":
				// TODO: add oauth-proxy image to image manifests
				continue
			case "alertmanager":
				imageKey = "prometheus_alertmanager"
			case "config-reloader":
				imageKey = "prometheus-config-reloader"
			}
			imageValue, exists := testImagemanifestsMap[imageKey]
			if !exists {
				t.Fatalf("The image key(%s) for the container(%s) doesn't exist in the statefulset(%s)", imageKey, container.Name, statefulName)
			}
			if imageValue == container.Image {
				t.Fatalf("The image(%s) for the container(%s) in the statefulset(%s) should not replace with the one in the image manifests", imageValue, container.Name, statefulName)
			}
		}
	}

	err = cl.Create(context.TODO(), newMCHInstanceWithVersion(config.GetMCONamespace(), version))
	if err != nil {
		t.Fatalf("Failed to create mch instance: (%v)", err)
	}

	// create reconcile request for MCH update event
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      config.MCHUpdatedRequestName,
			Namespace: config.GetMCONamespace(),
		},
	}

	// trigger another reconcile for MCH update event
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	//wait for update status
	time.Sleep(1 * time.Second)

	for _, deployName := range expectedDeploymentNames {
		deploy := &appsv1.Deployment{}
		err = cl.Get(context.TODO(), types.NamespacedName{
			Name:      deployName,
			Namespace: namespace,
		}, deploy)
		if err != nil {
			t.Fatalf("Failed to get deployment %s: %v", deployName, err)
		}
		for _, container := range deploy.Spec.Template.Spec.Containers {
			imageKey := strings.ReplaceAll(container.Name, "-", "_")
			switch container.Name {
			case "oauth-proxy":
				// TODO: add oauth-proxy image to image manifests
				continue
			case "config-reloader":
				imageKey = "prometheus-config-reloader"
			}
			imageValue, exists := testImagemanifestsMap[imageKey]
			if !exists {
				t.Fatalf("The image key(%s) for the container(%s) doesn't exist in the deployment(%s)", imageKey, container.Name, deployName)
			}
			if imageValue != container.Image {
				t.Fatalf("The image(%s) for the container(%s) in the deployment(%s) should not replace with the one in the image manifests", imageValue, container.Name, deployName)
			}
		}
	}

	for _, statefulName := range expectedStatefulSetNames {
		sts := &appsv1.StatefulSet{}
		err = cl.Get(context.TODO(), types.NamespacedName{
			Name:      statefulName,
			Namespace: namespace,
		}, sts)
		if err != nil {
			t.Fatalf("Failed to get statefulset %s: %v", statefulName, err)
		}
		for _, container := range sts.Spec.Template.Spec.Containers {
			imageKey := strings.ReplaceAll(container.Name, "-", "_")
			switch container.Name {
			case "oauth-proxy", "alertmanager-proxy":
				// TODO: add oauth-proxy image to image manifests
				continue
			case "alertmanager":
				imageKey = "prometheus_alertmanager"
			case "config-reloader":
				imageKey = "prometheus-config-reloader"
			}
			imageValue, exists := testImagemanifestsMap[imageKey]
			if !exists {
				t.Fatalf("The image key(%s) for the container(%s) doesn't exist in the statefulset(%s)", imageKey, container.Name, statefulName)
			}
			if imageValue != container.Image {
				t.Fatalf("The image(%s) for the container(%s) in the statefulset(%s) should not replace with the one in the image manifests", imageValue, container.Name, statefulName)
			}
		}
	}

	if err = os.Remove(testManifestsPath); err != nil {
		t.Fatalf("Failed to delete symbollink(%s) for the test manifests: (%v)", testManifestsPath, err)
	}
	os.Remove(path.Join(wd, "../../tests"))

	// stop update status routine
	stopStatusUpdate <- struct{}{}
	//wait for update status
	time.Sleep(1 * time.Second)
}

func createSecret(key, name, namespace string) *corev1.Secret {

	s3Conf := &config.ObjectStorgeConf{
		Type: "s3",
		Config: config.Config{
			Bucket:    "bucket",
			Endpoint:  "endpoint",
			Insecure:  true,
			AccessKey: "access_key",
			SecretKey: "secret_key`",
		},
	}
	configYaml, _ := yaml.Marshal(s3Conf)

	configYamlMap := map[string][]byte{}
	configYamlMap[key] = configYaml

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: "Opaque",
		Data: configYamlMap,
	}
}

func TestCheckObjStorageStatus(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
			},
		},
	}

	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	objs := []runtime.Object{mco}
	c := fake.NewFakeClient(objs...)
	mcoCondition := checkObjStorageStatus(c, mco)
	if mcoCondition == nil {
		t.Errorf("check s3 conf failed: got %v, expected non-nil", mcoCondition)
	}

	err := c.Create(context.TODO(), createSecret("test", "test", config.GetDefaultNamespace()))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}

	mcoCondition = checkObjStorageStatus(c, mco)
	if mcoCondition != nil {
		t.Errorf("check s3 conf failed: got %v, expected nil", mcoCondition)
	}

	updateSecret := createSecret("error", "test", config.GetDefaultNamespace())
	updateSecret.ObjectMeta.ResourceVersion = "1"
	err = c.Update(context.TODO(), updateSecret)
	if err != nil {
		t.Fatalf("Failed to update secret: (%v)", err)
	}

	mcoCondition = checkObjStorageStatus(c, mco)
	if mcoCondition == nil {
		t.Errorf("check s3 conf failed: got %v, expected no-nil", mcoCondition)
	}
}

func TestHandleStorageSizeChange(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				AlertmanagerStorageSize: "2Gi",
			},
		},
	}

	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	objs := []runtime.Object{
		mco,
		createStatefulSet(mco.Name, config.GetDefaultNamespace(), "test"),
		createPersistentVolumeClaim(mco.Name, config.GetDefaultNamespace(), "test"),
	}
	c := fake.NewFakeClient(objs...)
	r := &MultiClusterObservabilityReconciler{Client: c, Scheme: s}
	isAlertmanagerStorageSizeChanged = true
	r.HandleStorageSizeChange(mco)

	pvc := &corev1.PersistentVolumeClaim{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      "test",
		Namespace: config.GetDefaultNamespace(),
	}, pvc)

	if err == nil {
		if !pvc.Spec.Resources.Requests.Storage().Equal(resource.MustParse("2Gi")) {
			t.Errorf("update pvc failed: got %v, expected 2Gi", pvc.Spec.Resources.Requests.Storage())
		}
	} else {
		t.Errorf("update pvc failed: %v", err)
	}

}

func createStatefulSet(name, namespace, statefulSetName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
			Labels: map[string]string{
				"observability.open-cluster-management.io/name": name,
				"alertmanager": "observability",
			},
		},
	}
}

func createPersistentVolumeClaim(name, namespace, pvcName string) *corev1.PersistentVolumeClaim {
	storage := "gp2"
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"observability.open-cluster-management.io/name": name,
				"alertmanager": "observability",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storage,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("1Gi"),
				},
			},
		},
	}
}
