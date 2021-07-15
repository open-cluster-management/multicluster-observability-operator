// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	obsv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	ocinfrav1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcoshared "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	observabilityv1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

const (
	crLabelKey                        = "observability.open-cluster-management.io/name"
	clusterNameLabelKey               = "cluster"
	obsAPIGateway                     = "observatorium-api"
	infrastructureConfigName          = "cluster"
	defaultMCONamespace               = "open-cluster-management"
	defaultNamespace                  = "open-cluster-management-observability"
	defaultTenantName                 = "default"
	defaultCRName                     = "observability"
	operandNamePrefix                 = "observability-"
	OpenshiftIngressOperatorNamespace = "openshift-ingress-operator"
	OpenshiftIngressNamespace         = "openshift-ingress"
	OpenshiftIngressOperatorCRName    = "default"
	OpenshiftIngressDefaultCertName   = "router-certs-default"
	OpenshiftIngressRouteCAName       = "router-ca"

	AnnotationKeyImageRepository          = "mco-imageRepository"
	AnnotationKeyImageTagSuffix           = "mco-imageTagSuffix"
	AnnotationMCOPause                    = "mco-pause"
	AnnotationMCOWithoutResourcesRequests = "mco-thanos-without-resources-requests"
	AnnotationSkipCreation                = "skip-creation-if-exist"
	AnnotationCertDuration                = "mco-cert-duration"

	DefaultImgRepository   = "quay.io/open-cluster-management"
	DefaultDSImgRepository = "quay.io:443/acm-d"
	DefaultImgTagSuffix    = "latest"

	MCHUpdatedRequestName               = "mch-updated-request"
	ImageManifestConfigMapNamePrefix    = "mch-image-manifest-"
	OCMManifestConfigMapTypeLabelKey    = "ocm-configmap-type"
	OCMManifestConfigMapTypeLabelValue  = "image-manifest"
	OCMManifestConfigMapVersionLabelKey = "ocm-release-version"

	ComponentVersion = "COMPONENT_VERSION"

	ServerCACerts    = "observability-server-ca-certs"
	ClientCACerts    = "observability-client-ca-certs"
	ServerCerts      = "observability-server-certs"
	ServerCertCN     = "observability-server-certificate"
	GrafanaCerts     = "observability-grafana-certs"
	GrafanaCN        = "grafana"
	ManagedClusterOU = "acm"

	AlertmanagerAccessorSAName     = "observability-alertmanager-accessor"
	AlertmanagerAccessorSecretName = "observability-alertmanager-accessor"
	AlertmanagerServiceName        = "alertmanager"
	AlertmanagerRouteName          = "alertmanager"
	AlertmanagerRouteBYOCAName     = "alertmanager-byo-ca"
	AlertmanagerRouteBYOCERTName   = "alertmanager-byo-cert"

	AlertRuleDefaultConfigMapName = "thanos-ruler-default-rules"
	AlertRuleDefaultFileKey       = "default_rules.yaml"
	AlertRuleCustomConfigMapName  = "thanos-ruler-custom-rules"
	AlertRuleCustomFileKey        = "custom_rules.yaml"
	AlertmanagerURL               = "http://alertmanager:9093"
	AlertmanagerConfigName        = "alertmanager-config"

	AlertmanagersDefaultConfigMapName     = "thanos-ruler-config"
	AlertmanagersDefaultConfigFileKey     = "config.yaml"
	AlertmanagersDefaultCaBundleMountPath = "/etc/thanos/configmaps/alertmanager-ca-bundle"
	AlertmanagersDefaultCaBundleName      = "alertmanager-ca-bundle"
	AlertmanagersDefaultCaBundleKey       = "service-ca.crt"

	AllowlistConfigMapName       = "observability-metrics-allowlist"
	AllowlistCustomConfigMapName = "observability-metrics-custom-allowlist"

	ProxyServiceName      = "rbac-query-proxy"
	ProxyRouteName        = "rbac-query-proxy"
	ProxyRouteBYOCAName   = "proxy-byo-ca"
	ProxyRouteBYOCERTName = "proxy-byo-cert"
)

const (
	ObservatoriumImgRepo           = "quay.io/observatorium"
	ObservatoriumAPIImgName        = "observatorium"
	ObservatoriumAPIImgTag         = "2.3.0-SNAPSHOT-2021-06-30-00-51-08"
	ObservatoriumOperatorImgName   = "observatorium_operator"
	ThanosReceiveControllerImgName = "thanos-receive-controller"
	//ThanosReceiveControllerKey is used to get from mch-image-manifest.xxx configmap
	ThanosReceiveControllerKey    = "thanos_receive_controller"
	ThanosReceiveControllerImgTag = "master-2021-04-28-ee165b6"

	ThanosImgName = "thanos"
	ThanosImgTag  = "2.3.0-SNAPSHOT-2021-06-30-00-51-08"

	MemcachedImgRepo = "docker.io"
	MemcachedImgName = "memcached"
	MemcachedImgTag  = "1.6.3-alpine"

	MemcachedExporterImgRepo = "prom"
	MemcachedExporterImgName = "memcached-exporter"
	MemcachedExporterKey     = "memcached_exporter"
	MemcachedExporterImgTag  = "v0.9.0"

	GrafanaImgRepo            = "grafana"
	GrafanaImgName            = "grafana"
	GrafanaImgTagSuffix       = "7.4.2"
	GrafanaDashboardLoaderKey = "grafana_dashboard_loader"

	AlertManagerImgName           = "prometheus-alertmanager"
	AlertManagerImgKey            = "prometheus_alertmanager"
	ConfigmapReloaderImgRepo      = "quay.io/openshift"
	ConfigmapReloaderImgName      = "origin-configmap-reloader"
	ConfigmapReloaderImgTagSuffix = "4.5.0"
	ConfigmapReloaderKey          = "prometheus-config-reloader"

	OauthProxyImgRepo      = "quay.io/open-cluster-management"
	OauthProxyImgName      = "origin-oauth-proxy"
	OauthProxyImgTagSuffix = "2.0.11-SNAPSHOT-2021-04-29-18-29-17"
	OauthProxyKey          = "oauth_proxy"

	EndpointControllerImgTagSuffix = "2.2.0-6a5ea47fc39d51fb4fade6157843f2977442996e"
	EndpointControllerImgName      = "endpoint-monitoring-operator"
	EndpointControllerKey          = "endpoint_monitoring_operator"

	MetricsCollectorImgTagSuffix = "2.2.0-ff79e6ec8783756b942a77f08b3ab763dfd2dc15"
	MetricsCollectorImgName      = "metrics-collector"
	MetricsCollectorKey          = "metrics_collector"

	LeaseControllerImageTagSuffix = "2.1.0-a2899de5ce144e2c0441063e9ee8c4addf3ecb4a"
	LeaseControllerImageName      = "klusterlet-addon-lease-controller"
	LeaseControllerKey            = "klusterlet_addon_lease_controller"

	RBACQueryProxyKey = "rbac_query_proxy"

	RBACQueryProxyCPURequets    = "20m"
	RBACQueryProxyMemoryRequets = "100Mi"

	GrafanaCPURequets    = "4m"
	GrafanaMemoryRequets = "100Mi"
	GrafanaCPULimits     = "500m"
	GrafanaMemoryLimits  = "1Gi"

	AlertmanagerCPURequets    = "4m"
	AlertmanagerMemoryRequets = "200Mi"

	ObservatoriumAPICPURequets    = "20m"
	ObservatoriumAPIMemoryRequets = "128Mi"

	ThanosQueryFrontendCPURequets    = "100m"
	ThanosQueryFrontendMemoryRequets = "256Mi"

	MemcachedExporterCPURequets    = "5m"
	MemcachedExporterMemoryRequets = "50Mi"

	ThanosQueryCPURequets    = "300m"
	ThanosQueryMemoryRequets = "1Gi"

	ThanosCompactCPURequets    = "100m"
	ThanosCompactMemoryRequets = "512Mi"

	ObservatoriumReceiveControllerCPURequets    = "4m"
	ObservatoriumReceiveControllerMemoryRequets = "32Mi"

	ThanosReceiveCPURequets    = "300m"
	ThanosReceiveMemoryRequets = "512Mi"

	ThanosRuleCPURequets            = "50m"
	ThanosRuleMemoryRequets         = "512Mi"
	ThanosRuleReloaderCPURequets    = "4m"
	ThanosRuleReloaderMemoryRequets = "25Mi"

	ThanosCachedCPURequets            = "45m"
	ThanosCachedMemoryRequets         = "128Mi"
	ThanosCachedExporterCPURequets    = "5m"
	ThanosCachedExporterMemoryRequets = "50Mi"

	ThanosStoreCPURequets    = "100m"
	ThanosStoreMemoryRequets = "1Gi"

	MetricsCollectorCPURequets    = "100m"
	MetricsCollectorMemoryRequets = "100Mi"
	MetricsCollectorCPULimits     = ""
	MetricsCollectorMemoryLimits  = ""

	ObservatoriumAPI             = "observatorium-api"
	ThanosCompact                = "thanos-compact"
	ThanosQuery                  = "thanos-query"
	ThanosQueryFrontend          = "thanos-query-frontend"
	ThanosQueryFrontendMemcached = "thanos-query-frontend-memcached"
	ThanosRule                   = "thanos-rule"
	ThanosReceive                = "thanos-receive-default"
	ThanosStoreMemcached         = "thanos-store-memcached"
	ThanosStoreShard             = "thanos-store-shard"
	MemcachedExporter            = "memcached-exporter"
	Grafana                      = "grafana"
	RBACQueryProxy               = "rbac-query-proxy"
	Alertmanager                 = "alertmanager"
	ThanosReceiveController      = "thanos-receive-controller"
	ObservatoriumOperator        = "observatorium-operator"
	MetricsCollector             = "metrics-collector"
	Observatorium                = "observatorium"

	RetentionResolutionRaw = "30d"
	RetentionResolution5m  = "180d"
	RetentionResolution1h  = "0d"
	RetentionInLocal       = "24h"
	DeleteDelay            = "48h"
	BlockDuration          = "2h"

	DefaultImagePullPolicy = "Always"
	DefaultImagePullSecret = "multiclusterhub-operator-pull-secret"

	ResourceLimits   = "limits"
	ResourceRequests = "requests"
)

const (
	MCORsName = "multiclusterobservabilities"
)

const (
	MCHCrdName                     = "multiclusterhubs.operator.open-cluster-management.io"
	MCOCrdName                     = "multiclusterobservabilities.observability.open-cluster-management.io"
	StorageVersionMigrationCrdName = "storageversionmigrations.migration.k8s.io"
)

// ObjectStorgeConf is used to Unmarshal from bytes to do validation
type ObjectStorgeConf struct {
	Type   string `yaml:"type"`
	Config Config `yaml:"config"`
}

var (
	log                         = logf.Log.WithName("config")
	monitoringCRName            = ""
	tenantUID                   = ""
	imageManifests              = map[string]string{}
	imageManifestConfigMapName  = ""
	hasCustomRuleConfigMap      = false
	hasCustomAlertmanagerConfig = false
	certDuration                = time.Hour * 24 * 365

	Replicas1 int32 = 1
	Replicas2 int32 = 2
	Replicas3 int32 = 3
	Replicas        = map[string]*int32{
		ObservatoriumAPI:    &Replicas2,
		ThanosQuery:         &Replicas2,
		ThanosQueryFrontend: &Replicas2,
		Grafana:             &Replicas2,
		RBACQueryProxy:      &Replicas2,

		ThanosRule:                   &Replicas3,
		ThanosReceive:                &Replicas3,
		ThanosStoreShard:             &Replicas3,
		ThanosStoreMemcached:         &Replicas3,
		ThanosQueryFrontendMemcached: &Replicas3,
		Alertmanager:                 &Replicas3,
	}
	// use this map to store the operand name
	operandNames = map[string]string{}

	MemoryLimitMB   = int32(1024)
	ConnectionLimit = int32(1024)
	MaxItemSize     = "1m"
)

func GetReplicas(component string, advanced *observabilityv1beta2.AdvancedConfig) *int32 {
	if advanced == nil {
		return Replicas[component]
	}
	var replicas *int32
	switch component {
	case ObservatoriumAPI:
		if advanced.ObservatoriumAPI != nil {
			replicas = advanced.ObservatoriumAPI.Replicas
		}
	case ThanosQuery:
		if advanced.Query != nil {
			replicas = advanced.Query.Replicas
		}
	case ThanosQueryFrontend:
		if advanced.QueryFrontend != nil {
			replicas = advanced.QueryFrontend.Replicas
		}
	case ThanosQueryFrontendMemcached:
		if advanced.QueryFrontendMemcached != nil {
			replicas = advanced.QueryFrontendMemcached.CommonSpec.Replicas
		}
	case ThanosRule:
		if advanced.Rule != nil {
			replicas = advanced.Rule.Replicas
		}
	case ThanosReceive:
		if advanced.Receive != nil {
			replicas = advanced.Receive.Replicas
		}
	case ThanosStoreMemcached:
		if advanced.StoreMemcached != nil {
			replicas = advanced.StoreMemcached.CommonSpec.Replicas
		}
	case ThanosStoreShard:
		if advanced.Store != nil {
			replicas = advanced.Store.Replicas
		}
	case RBACQueryProxy:
		if advanced.RBACQueryProxy != nil {
			replicas = advanced.RBACQueryProxy.Replicas
		}
	case Grafana:
		if advanced.Grafana != nil {
			replicas = advanced.Grafana.Replicas
		}
	case Alertmanager:
		if advanced.Alertmanager != nil {
			replicas = advanced.Alertmanager.Replicas
		}
	}
	if replicas == nil || *replicas == 0 {
		replicas = Replicas[component]
	}
	return replicas
}

// GetCrLabelKey returns the key for the CR label injected into the resources created by the operator
func GetCrLabelKey() string {
	return crLabelKey
}

// GetClusterNameLabelKey returns the key for the injected label
func GetClusterNameLabelKey() string {
	return clusterNameLabelKey
}

func GetImageManifestConfigMapName() string {
	return imageManifestConfigMapName
}

// ReadImageManifestConfigMap reads configmap with the label ocm-configmap-type=image-manifest
func ReadImageManifestConfigMap(c client.Client, version string) (bool, error) {
	mcoNamespace := GetMCONamespace()
	// List image manifest configmap with label ocm-configmap-type=image-manifest and ocm-release-version
	matchLabels := map[string]string{
		OCMManifestConfigMapTypeLabelKey:    OCMManifestConfigMapTypeLabelValue,
		OCMManifestConfigMapVersionLabelKey: version,
	}
	listOpts := []client.ListOption{
		client.InNamespace(mcoNamespace),
		client.MatchingLabels(matchLabels),
	}

	imageCMList := &corev1.ConfigMapList{}
	err := c.List(context.TODO(), imageCMList, listOpts...)
	if err != nil {
		return false, fmt.Errorf("Failed to list mch-image-manifest configmaps: %v", err)
	}

	if len(imageCMList.Items) != 1 {
		// there should be only one matched image manifest configmap found
		return false, nil
	}

	imageManifests = imageCMList.Items[0].Data
	return true, nil
}

// GetImageManifests...
func GetImageManifests() map[string]string {
	return imageManifests
}

// SetImageManifests sets imageManifests
func SetImageManifests(images map[string]string) {
	imageManifests = images
}

// ReplaceImage is used to replace the image with specified annotation or imagemanifest configmap
func ReplaceImage(annotations map[string]string, imageRepo, componentName string) (bool, string) {
	if annotations != nil {
		annotationImageRepo, _ := annotations[AnnotationKeyImageRepository]
		if annotationImageRepo == "" {
			annotationImageRepo = DefaultImgRepository
		}
		// This is for test only. e.g.:
		// if there is "mco-metrics_collector-image" defined in annotation, use it for testing
		componentImage, hasComponentImage := annotations["mco-"+componentName+"-image"]
		tagSuffix, hasTagSuffix := annotations[AnnotationKeyImageTagSuffix]
		sameOrg := strings.Contains(imageRepo, DefaultImgRepository)

		if hasComponentImage {
			return true, componentImage
		} else if hasTagSuffix && sameOrg {
			repoSlice := strings.Split(imageRepo, "/")
			imageName := strings.Split(repoSlice[len(repoSlice)-1], ":")[0]
			image := annotationImageRepo + "/" + imageName + ":" + tagSuffix
			return true, image
		} else if !hasTagSuffix {
			image, found := imageManifests[componentName]
			if found {
				return true, image
			}
			return false, ""
		}
		return false, ""
	} else {
		image, found := imageManifests[componentName]
		if found {
			return true, image
		}
		return false, ""
	}
}

// GetDefaultTenantName returns the default tenant name
func GetDefaultTenantName() string {
	return defaultTenantName
}

// GetObsAPIUrl is used to get the URL for observartium api gateway
func GetObsAPIUrl(client client.Client, namespace string) (string, error) {
	found := &routev1.Route{}

	err := client.Get(context.TODO(), types.NamespacedName{Name: obsAPIGateway, Namespace: namespace}, found)
	if err != nil {
		return "", err
	}
	return found.Spec.Host, nil
}

func GetMCONamespace() string {
	podNamespace, found := os.LookupEnv("POD_NAMESPACE")
	if !found {
		podNamespace = defaultMCONamespace
	}
	return podNamespace
}

// GetAlertmanagerEndpoint is used to get the URL for alertmanager
func GetAlertmanagerEndpoint(client client.Client, namespace string) (string, error) {
	found := &routev1.Route{}

	err := client.Get(context.TODO(), types.NamespacedName{Name: AlertmanagerRouteName, Namespace: namespace}, found)
	if err != nil {
		return "", err
	}
	return found.Spec.Host, nil
}

// GetAlertmanagerRouterCA is used to get the CA of openshift Route
func GetAlertmanagerRouterCA(client client.Client) (string, error) {
	amRouteBYOCaSrt := &corev1.Secret{}
	amRouteBYOCertSrt := &corev1.Secret{}
	err1 := client.Get(context.TODO(), types.NamespacedName{Name: AlertmanagerRouteBYOCAName, Namespace: GetDefaultNamespace()}, amRouteBYOCaSrt)
	err2 := client.Get(context.TODO(), types.NamespacedName{Name: AlertmanagerRouteBYOCERTName, Namespace: GetDefaultNamespace()}, amRouteBYOCertSrt)
	if err1 == nil && err2 == nil {
		return string(amRouteBYOCaSrt.Data["tls.crt"]), nil
	}

	ingressOperator := &operatorv1.IngressController{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: OpenshiftIngressOperatorCRName, Namespace: OpenshiftIngressOperatorNamespace}, ingressOperator)
	if err != nil {
		return "", err
	}

	routerCASrtName := OpenshiftIngressDefaultCertName
	// check if custom default certificate is provided or not
	if ingressOperator.Spec.DefaultCertificate != nil {
		routerCASrtName = ingressOperator.Spec.DefaultCertificate.Name
	}

	routerCASecret := &corev1.Secret{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: routerCASrtName, Namespace: OpenshiftIngressNamespace}, routerCASecret)
	if err != nil {
		return "", err
	}
	return string(routerCASecret.Data["tls.crt"]), nil
}

func GetDefaultNamespace() string {
	return defaultNamespace
}

// GetMonitoringCRName returns monitoring cr name
func GetMonitoringCRName() string {
	return monitoringCRName
}

// SetMonitoringCRName sets the cr name
func SetMonitoringCRName(crName string) {
	monitoringCRName = crName
}

func infrastructureConfigNameNsN() types.NamespacedName {
	return types.NamespacedName{
		Name: infrastructureConfigName,
	}
}

// GetKubeAPIServerAddress is used to get the api server url
func GetKubeAPIServerAddress(client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := client.Get(context.TODO(), infrastructureConfigNameNsN(), infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// GetClusterID is used to get the cluster uid
func GetClusterID(ocpClient ocpClientSet.Interface) (string, error) {
	clusterVersion, err := ocpClient.ConfigV1().ClusterVersions().Get(context.TODO(), "version", v1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get clusterVersion")
		return "", err
	}

	return string(clusterVersion.Spec.ClusterID), nil
}

// checkIsIBMCloud detects if the current cloud vendor is ibm or not
// we know we are on OCP already, so if it's also ibm cloud, it's roks
func CheckIsIBMCloud(c client.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	err := c.List(context.TODO(), nodes)
	if err != nil {
		log.Error(err, "Failed to get nodes list")
		return false, err
	}
	if len(nodes.Items) == 0 {
		log.Error(err, "Failed to list any nodes")
		return false, nil
	}

	providerID := nodes.Items[0].Spec.ProviderID
	if strings.Contains(providerID, "ibm") {
		return true, nil
	}

	return false, nil
}

// GetDefaultCRName is used to get default CR name.
func GetDefaultCRName() string {
	return defaultCRName
}

// IsPaused returns true if the multiclusterobservability instance is labeled as paused, and false otherwise
func IsPaused(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	if annotations[AnnotationMCOPause] != "" &&
		strings.EqualFold(annotations[AnnotationMCOPause], "true") {
		return true
	}

	return false
}

// WithoutResourcesRequests returns true if the multiclusterobservability instance has annotation:
// mco-thanos-without-resources-requests: "true"
// This is just for test purpose: the KinD cluster does not have enough resources for the requests.
// We won't expose this annotation to the customer.
func WithoutResourcesRequests(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	if annotations[AnnotationMCOWithoutResourcesRequests] != "" &&
		strings.EqualFold(annotations[AnnotationMCOWithoutResourcesRequests], "true") {
		return true
	}

	return false
}

// GetTenantUID returns tenant uid
func GetTenantUID() string {
	if tenantUID == "" {
		tenantUID = string(uuid.NewUUID())
	}
	return tenantUID
}

// GetObsAPISvc returns observatorium api service
func GetObsAPISvc(instanceName string) string {
	return instanceName + "-observatorium-api." + defaultNamespace + ".svc.cluster.local"
}

// SetCustomRuleConfigMap set true if there is custom rule configmap
func SetCustomRuleConfigMap(hasConfigMap bool) {
	hasCustomRuleConfigMap = hasConfigMap
}

// HasCustomRuleConfigMap returns true if there is custom rule configmap
func HasCustomRuleConfigMap() bool {
	return hasCustomRuleConfigMap
}

func GetCertDuration() time.Duration {
	return certDuration
}

func SetCertDuration(annotations map[string]string) {
	if annotations != nil && annotations[AnnotationCertDuration] != "" {
		d, err := time.ParseDuration(annotations[AnnotationCertDuration])
		if err != nil {
			log.Error(err, "Failed to parse cert duration, use default one", "annotation", annotations[AnnotationCertDuration])
		} else {
			certDuration = d
			return
		}
	}
	certDuration = time.Hour * 24 * 365
}

func GetOperandNamePrefix() string {
	return operandNamePrefix
}

func GetImagePullPolicy(mco observabilityv1beta2.MultiClusterObservabilitySpec) corev1.PullPolicy {
	if mco.ImagePullPolicy != "" {
		return mco.ImagePullPolicy
	} else {
		return DefaultImagePullPolicy
	}
}

func GetImagePullSecret(mco observabilityv1beta2.MultiClusterObservabilitySpec) string {
	if mco.ImagePullSecret != "" {
		return mco.ImagePullSecret
	} else {
		return DefaultImagePullSecret
	}
}

func getDefaultResource(resourceType string, resource corev1.ResourceName,
	component string) string {
	//No provide the default limits
	if resourceType == ResourceLimits && component != Grafana {
		return ""
	}
	switch component {
	case ObservatoriumAPI:
		if resource == corev1.ResourceCPU {
			return ObservatoriumAPICPURequets
		}
		if resource == corev1.ResourceMemory {
			return ObservatoriumAPIMemoryRequets
		}
	case ThanosCompact:
		if resource == corev1.ResourceCPU {
			return ThanosCompactCPURequets
		}
		if resource == corev1.ResourceMemory {
			return ThanosCompactMemoryRequets
		}
	case ThanosQuery:
		if resource == corev1.ResourceCPU {
			return ThanosQueryCPURequets
		}
		if resource == corev1.ResourceMemory {
			return ThanosQueryMemoryRequets
		}
	case ThanosQueryFrontend:
		if resource == corev1.ResourceCPU {
			return ThanosQueryFrontendCPURequets
		}
		if resource == corev1.ResourceMemory {
			return ThanosQueryFrontendMemoryRequets
		}
	case ThanosRule:
		if resource == corev1.ResourceCPU {
			return ThanosRuleCPURequets
		}
		if resource == corev1.ResourceMemory {
			return ThanosRuleMemoryRequets
		}
	case ThanosReceive:
		if resource == corev1.ResourceCPU {
			return ThanosReceiveCPURequets
		}
		if resource == corev1.ResourceMemory {
			return ThanosReceiveMemoryRequets
		}
	case ThanosStoreShard:
		if resource == corev1.ResourceCPU {
			return ThanosStoreCPURequets
		}
		if resource == corev1.ResourceMemory {
			return ThanosStoreMemoryRequets
		}
	case ThanosQueryFrontendMemcached, ThanosStoreMemcached:
		if resource == corev1.ResourceCPU {
			return ThanosCachedCPURequets
		}
		if resource == corev1.ResourceMemory {
			return ThanosCachedMemoryRequets
		}
	case MemcachedExporter:
		if resource == corev1.ResourceCPU {
			return MemcachedExporterCPURequets
		}
		if resource == corev1.ResourceMemory {
			return MemcachedExporterMemoryRequets
		}
	case RBACQueryProxy:
		if resource == corev1.ResourceCPU {
			return RBACQueryProxyCPURequets
		}
		if resource == corev1.ResourceMemory {
			return RBACQueryProxyMemoryRequets
		}
	case MetricsCollector:
		if resource == corev1.ResourceCPU {
			return MetricsCollectorCPURequets
		}
		if resource == corev1.ResourceMemory {
			return MetricsCollectorMemoryRequets
		}
	case Alertmanager:
		if resource == corev1.ResourceCPU {
			return AlertmanagerCPURequets
		}
		if resource == corev1.ResourceMemory {
			return AlertmanagerMemoryRequets
		}
	case Grafana:
		if resourceType == ResourceRequests {
			if resource == corev1.ResourceCPU {
				return GrafanaCPURequets
			}
			if resource == corev1.ResourceMemory {
				return GrafanaMemoryRequets
			}
		} else if resourceType == ResourceLimits {
			if resource == corev1.ResourceCPU {
				return GrafanaCPULimits
			}
			if resource == corev1.ResourceMemory {
				return GrafanaMemoryLimits
			}
		}
	}
	return ""
}

func getResource(resourceType string, resource corev1.ResourceName,
	component string, advanced *observabilityv1beta2.AdvancedConfig) string {
	if advanced == nil {
		return getDefaultResource(resourceType, resource, component)
	}
	var resourcesReq *corev1.ResourceRequirements
	switch component {
	case ObservatoriumAPI:
		if advanced.ObservatoriumAPI != nil {
			resourcesReq = advanced.ObservatoriumAPI.Resources
		}
	case ThanosCompact:
		if advanced.Compact != nil {
			resourcesReq = advanced.Compact.Resources
		}
	case ThanosQuery:
		if advanced.Query != nil {
			resourcesReq = advanced.Query.Resources
		}
	case ThanosQueryFrontend:
		if advanced.QueryFrontend != nil {
			resourcesReq = advanced.QueryFrontend.Resources
		}
	case ThanosQueryFrontendMemcached:
		if advanced.QueryFrontendMemcached != nil {
			resourcesReq = advanced.QueryFrontendMemcached.CommonSpec.Resources
		}
	case ThanosRule:
		if advanced.Rule != nil {
			resourcesReq = advanced.Rule.Resources
		}
	case ThanosReceive:
		if advanced.Receive != nil {
			resourcesReq = advanced.Receive.Resources
		}
	case ThanosStoreMemcached:
		if advanced.StoreMemcached != nil {
			resourcesReq = advanced.StoreMemcached.CommonSpec.Resources
		}
	case ThanosStoreShard:
		if advanced.Store != nil {
			resourcesReq = advanced.Store.Resources
		}
	case RBACQueryProxy:
		if advanced.RBACQueryProxy != nil {
			resourcesReq = advanced.RBACQueryProxy.Resources
		}
	case Grafana:
		if advanced.Grafana != nil {
			resourcesReq = advanced.Grafana.Resources
		}
	case Alertmanager:
		if advanced.Alertmanager != nil {
			resourcesReq = advanced.Alertmanager.Resources
		}
	}

	if resourcesReq != nil {
		if resourceType == ResourceRequests {
			if len(resourcesReq.Requests) != 0 {
				if resource == corev1.ResourceCPU {
					return resourcesReq.Requests.Cpu().String()
				} else if resource == corev1.ResourceMemory {
					return resourcesReq.Requests.Memory().String()
				} else {
					return getDefaultResource(resourceType, resource, component)
				}
			} else {
				return getDefaultResource(resourceType, resource, component)
			}
		}
		if resourceType == ResourceLimits {
			if len(resourcesReq.Limits) != 0 {
				if resource == corev1.ResourceCPU {
					return resourcesReq.Limits.Cpu().String()
				} else if resource == corev1.ResourceMemory {
					return resourcesReq.Limits.Memory().String()
				} else {
					return getDefaultResource(resourceType, resource, component)
				}
			} else {
				return getDefaultResource(resourceType, resource, component)
			}
		}
	} else {
		return getDefaultResource(resourceType, resource, component)
	}
	return ""
}

func GetResources(component string, advanced *observabilityv1beta2.AdvancedConfig) corev1.ResourceRequirements {

	cpuRequests := getResource(ResourceRequests, corev1.ResourceCPU, component, advanced)
	cpuLimits := getResource(ResourceLimits, corev1.ResourceCPU, component, advanced)
	memoryRequests := getResource(ResourceRequests, corev1.ResourceMemory, component, advanced)
	memoryLimits := getResource(ResourceLimits, corev1.ResourceMemory, component, advanced)

	resourceReq := corev1.ResourceRequirements{}
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	if cpuRequests == "0" {
		cpuRequests = getDefaultResource(ResourceRequests, corev1.ResourceCPU, component)
	}
	if cpuRequests != "" {
		requests[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuRequests)
	}

	if memoryRequests == "0" {
		memoryRequests = getDefaultResource(ResourceRequests, corev1.ResourceMemory, component)
	}
	if memoryRequests != "" {
		requests[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryRequests)
	}

	if cpuLimits == "0" {
		cpuLimits = getDefaultResource(ResourceLimits, corev1.ResourceCPU, component)
	}
	if cpuLimits != "" {
		limits[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuLimits)
	}

	if memoryLimits == "0" {
		memoryLimits = getDefaultResource(ResourceLimits, corev1.ResourceMemory, component)
	}
	if memoryLimits != "" {
		limits[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryLimits)
	}
	resourceReq.Limits = limits
	resourceReq.Requests = requests

	return resourceReq
}

func GetOBAResources(oba *mcoshared.ObservabilityAddonSpec) *corev1.ResourceRequirements {
	cpuRequests := MetricsCollectorCPURequets
	cpuLimits := MetricsCollectorCPULimits
	memoryRequests := MetricsCollectorMemoryRequets
	memoryLimits := MetricsCollectorMemoryLimits

	if oba.Resources != nil {
		if len(oba.Resources.Requests) != 0 {
			if oba.Resources.Requests.Cpu().String() != "0" {
				cpuRequests = oba.Resources.Requests.Cpu().String()
			}
			if oba.Resources.Requests.Memory().String() != "0" {
				memoryRequests = oba.Resources.Requests.Memory().String()
			}
		}
		if len(oba.Resources.Limits) != 0 {
			if oba.Resources.Limits.Cpu().String() != "0" {
				cpuLimits = oba.Resources.Limits.Cpu().String()
			}
			if oba.Resources.Limits.Memory().String() != "0" {
				memoryLimits = oba.Resources.Limits.Memory().String()
			}
		}
	}

	resourceReq := &corev1.ResourceRequirements{}
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	if cpuRequests != "" {
		requests[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuRequests)
	}
	if memoryRequests != "" {
		requests[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryRequests)
	}
	if cpuLimits != "" {
		limits[corev1.ResourceName(corev1.ResourceCPU)] = resource.MustParse(cpuLimits)
	}
	if memoryLimits != "" {
		limits[corev1.ResourceName(corev1.ResourceMemory)] = resource.MustParse(memoryLimits)
	}
	resourceReq.Limits = limits
	resourceReq.Requests = requests

	return resourceReq
}

func GetOperandName(name string) string {
	log.V(1).Info("operand is", "key", name, "name", operandNames[name])
	return operandNames[name]
}

func SetOperandNames(c client.Client) error {
	if len(operandNames) != 0 {
		return nil
	}
	//set the default values.
	operandNames[Grafana] = GetOperandNamePrefix() + Grafana
	operandNames[RBACQueryProxy] = GetOperandNamePrefix() + RBACQueryProxy
	operandNames[Alertmanager] = GetOperandNamePrefix() + Alertmanager
	operandNames[ObservatoriumOperator] = GetOperandNamePrefix() + ObservatoriumOperator
	operandNames[Observatorium] = GetDefaultCRName()
	operandNames[ObservatoriumAPI] = GetOperandNamePrefix() + ObservatoriumAPI

	// Check if the Observatorium CR already exists
	opts := &client.ListOptions{
		Namespace: GetDefaultNamespace(),
	}

	observatoriumList := &obsv1alpha1.ObservatoriumList{}
	err := c.List(context.TODO(), observatoriumList, opts)
	if err != nil {
		return err
	}
	if len(observatoriumList.Items) != 0 {
		for _, observatorium := range observatoriumList.Items {
			for _, ownerRef := range observatorium.OwnerReferences {
				if ownerRef.Kind == "MultiClusterObservability" && ownerRef.Name == GetMonitoringCRName() {
					if observatorium.Name != GetDefaultCRName() {
						// this is for upgrade case.
						operandNames[Grafana] = Grafana
						operandNames[RBACQueryProxy] = RBACQueryProxy
						operandNames[Alertmanager] = Alertmanager
						operandNames[ObservatoriumOperator] = ObservatoriumOperator
						operandNames[Observatorium] = observatorium.Name
						operandNames[ObservatoriumAPI] = observatorium.Name + "-" + ObservatoriumAPI
					}
					break
				}
			}
		}
	}

	return nil
}
