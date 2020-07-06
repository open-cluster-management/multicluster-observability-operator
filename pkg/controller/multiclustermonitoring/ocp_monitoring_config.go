// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"
	"strings"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	manifests "github.com/openshift/cluster-monitoring-operator/pkg/manifests"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	clusterIDLabelKey = "cluster_id"
	collectorType     = "OCP_PROMETHEUS"
	cmName            = "cluster-monitoring-config"
	cmNamespace       = "openshift-monitoring"
	configKey         = "config.yaml"
	labelValue        = "hub_cluster"
	protocol          = "http://"
	urlSubPath        = "/api/metrics/v1/write"
)

func getConfigMap(client client.Client) (*v1.ConfigMap, error) {

	found := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: cmNamespace}, found)
	if err != nil {
		return nil, err
	}
	return found, err
}

func createRemoteWriteSpec(
	ocpClient ocpClientSet.Interface, url string,
	labelConfigs *[]monv1.RelabelConfig) (*monv1.RemoteWriteSpec, error) {

	if labelConfigs == nil {
		return nil, nil
	}
	clusterID, err := config.GetClusterID(ocpClient)
	if err != nil {
		return nil, err
	}

	requiredMetics := []string{
		"cluster_version",
		"cluster:capacity_cpu_cores:sum",
		"cluster:capacity_memory_bytes:sum",
		"cluster_version_payload",
		"cluster_infrastructure_provider",
		"kube_pod_container_resource_requests_memory_bytes",
		"machine_memory_bytes",
		"kube_pod_container_resource_requests_cpu_cores",
		"machine_cpu_cores",
		"cluster:usage:resources:sum",
		"cluster:cpu_usage_cores:sum",
		"cluster:memory_usage_bytes:sum",
		"cluster:container_cpu_usage:ratio",
		"cluster:container_spec_cpu_shares:ratio",
		"cluster:memory_usage:ratio",
		"kube_node_status_allocatable_memory_bytes",
		"haproxy_backend_connections_total",
	}

	relabelConfigs := []monv1.RelabelConfig{
		monv1.RelabelConfig{
			SourceLabels: []string{"__name__"},
			TargetLabel:  clusterIDLabelKey,
			Replacement:  clusterID,
		},

		monv1.RelabelConfig{
			Action:       "keep",
			SourceLabels: []string{"__name__"},
			Regex:        strings.Join(requiredMetics, "|"),
		},
	}

	newlabelConfigs := append(*labelConfigs, relabelConfigs...)
	if !strings.HasPrefix(url, "http") {
		url = protocol + url
	}

	if !strings.HasSuffix(url, urlSubPath) {
		url = url + urlSubPath
	}

	return &monv1.RemoteWriteSpec{
		URL:                 url,
		WriteRelabelConfigs: newlabelConfigs,
	}, nil
}

func createConfigMap(
	client client.Client,
	ocpClient ocpClientSet.Interface,
	url string, labelConfigs *[]monv1.RelabelConfig) error {

	rwSpec, err := createRemoteWriteSpec(ocpClient, url, labelConfigs)
	if err != nil {
		return err
	}
	config := &manifests.Config{
		PrometheusK8sConfig: &manifests.PrometheusK8sConfig{
			RemoteWrite: []monv1.RemoteWriteSpec{
				*rwSpec,
			},
		},
	}
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cmNamespace,
		},
		Data: map[string]string{configKey: string(configYaml)},
	}

	err = client.Create(context.TODO(), cm)
	if err == nil {
		log.Info("Configmap cluster-monitoring-config created")
	}
	return err
}

func updateConfigMap(
	client client.Client,
	ocpClient ocpClientSet.Interface,
	configmap *v1.ConfigMap,
	url string,
	labelConfigs *[]monv1.RelabelConfig) error {

	configYaml := configmap.Data[configKey]
	config := &manifests.Config{}
	err := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(configYaml), 100).Decode(&config)
	if err != nil {
		return err
	}
	rwSpec, err := createRemoteWriteSpec(ocpClient, url, labelConfigs)
	if err != nil {
		return err
	}
	if config.PrometheusK8sConfig == nil {
		if labelConfigs == nil {
			return nil
		}
		config.PrometheusK8sConfig = &manifests.PrometheusK8sConfig{}
	}
	if config.PrometheusK8sConfig.RemoteWrite == nil || len(config.PrometheusK8sConfig.RemoteWrite) == 0 {
		if labelConfigs == nil {
			return nil
		}
		config.PrometheusK8sConfig.RemoteWrite = []monv1.RemoteWriteSpec{
			*rwSpec,
		}
	} else {
		flag := false
		specs := []monv1.RemoteWriteSpec{}
		for _, spec := range config.PrometheusK8sConfig.RemoteWrite {
			if !strings.Contains(spec.URL, url) {
				specs = append(specs, spec)
			} else {
				if labelConfigs != nil {
					flag = true
					specs = append(specs, *rwSpec)
				}
				break
			}
		}
		if !flag && labelConfigs != nil {
			specs = append(specs, *rwSpec)
		}
		config.PrometheusK8sConfig.RemoteWrite = specs
	}
	updateConfigYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	configmap.Data[configKey] = string(updateConfigYaml)
	err = client.Update(context.TODO(), configmap)
	if err == nil {
		log.Info("Configmap updated")
	}
	return err
}

// updateClusterMonitoringConfig is used to update cluster-monitoring-config configmap on spoke clusters
func updateClusterMonitoringConfig(
	client client.Client, ocpClient ocpClientSet.Interface,
	url string, labelConfigs *[]monv1.RelabelConfig) error {

	cm, err := getConfigMap(client)
	if err != nil {
		if errors.IsNotFound(err) {
			if labelConfigs == nil {
				log.Info("No cluster-monitoring-config configmap found")
				return nil
			}
			err = createConfigMap(client, ocpClient, url, labelConfigs)
			return err
		}
		return err
	}
	err = updateConfigMap(client, ocpClient, cm, url, labelConfigs)
	return err
}

// UpdateHubClusterMonitoringConfig is used to cluster-monitoring-config configmap on hub clusters
func UpdateHubClusterMonitoringConfig(
	client client.Client,
	ocpClient ocpClientSet.Interface,
	namespace string) (*reconcile.Result, error) {

	url, err := config.GetObsAPIUrl(client, namespace)
	if err != nil {
		return &reconcile.Result{}, err
	}

	labelConfigs := []monv1.RelabelConfig{
		{
			SourceLabels: []string{"__name__"},
			TargetLabel:  config.GetClusterNameLabelKey(),
			Replacement:  labelValue,
		},
	}
	return nil, updateClusterMonitoringConfig(client, ocpClient, url, &labelConfigs)
}
