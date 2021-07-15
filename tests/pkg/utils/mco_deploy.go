// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/klog"
)

const (
	MCO_CR_NAME                   = "observability"
	MCO_COMPONENT_LABEL           = "observability.open-cluster-management.io/name=" + MCO_CR_NAME
	OBSERVATORIUM_COMPONENT_LABEL = "app.kubernetes.io/part-of=observatorium"
	MCO_NAMESPACE                 = "open-cluster-management-observability"
	MCO_ADDON_NAMESPACE           = "open-cluster-management-addon-observability"
	MCO_PULL_SECRET_NAME          = "multiclusterhub-operator-pull-secret"
	OBJ_SECRET_NAME               = "thanos-object-storage"
	MCO_GROUP                     = "observability.open-cluster-management.io"
	OCM_WORK_GROUP                = "work.open-cluster-management.io"
	OCM_CLUSTER_GROUP             = "cluster.open-cluster-management.io"
	OCM_ADDON_GROUP               = "addon.open-cluster-management.io"
)

func NewMCOGVRV1BETA1() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    MCO_GROUP,
		Version:  "v1beta1",
		Resource: "multiclusterobservabilities"}
}

func NewMCOGVRV1BETA2() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    MCO_GROUP,
		Version:  "v1beta2",
		Resource: "multiclusterobservabilities"}
}

func NewMCOAddonGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    MCO_GROUP,
		Version:  "v1beta1",
		Resource: "observabilityaddons"}
}

func NewOCMManifestworksGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    OCM_WORK_GROUP,
		Version:  "v1",
		Resource: "manifestworks"}
}

func NewOCMManagedClustersGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    OCM_CLUSTER_GROUP,
		Version:  "v1",
		Resource: "managedclusters"}
}

func NewMCOClusterManagementAddonsGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    OCM_ADDON_GROUP,
		Version:  "v1alpha1",
		Resource: "clustermanagementaddons"}
}

func NewMCOManagedClusterAddonsGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    OCM_ADDON_GROUP,
		Version:  "v1alpha1",
		Resource: "managedclusteraddons"}
}

func NewMCOMObservatoriumGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "core.observatorium.io",
		Version:  "v1alpha1",
		Resource: "observatoria"}
}

func NewOCMMultiClusterHubGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "operator.open-cluster-management.io",
		Version:  "v1",
		Resource: "multiclusterhubs"}
}

func ModifyMCOAvailabilityConfig(opt TestOptions, availabilityConfig string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}

	spec := mco.Object["spec"].(map[string]interface{})
	spec["availabilityConfig"] = availabilityConfig
	_, updateErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Update(context.TODO(), mco, metav1.UpdateOptions{})
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func GetAllMCOPods(opt TestOptions) ([]corev1.Pod, error) {
	hubClient := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	podList, err := hubClient.CoreV1().Pods(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return []corev1.Pod{}, err
	}

	// ignore non-mco pods
	mcoPods := []corev1.Pod{}
	for _, p := range podList.Items {
		if strings.Contains(p.GetName(), "grafana-test") {
			continue
		}

		if strings.Contains(p.GetName(), "minio") {
			continue
		}

		mcoPods = append(mcoPods, p)
	}

	return mcoPods, nil
}

func PrintAllMCOPodsStatus(opt TestOptions) {
	podList, err := GetAllMCOPods(opt)
	if err != nil {
		klog.Errorf("Failed to get all MCO pods")
	}

	if len(podList) == 0 {
		klog.V(1).Infof("Failed to get pod in <%s> namespace", MCO_NAMESPACE)
	}

	klog.V(1).Infof("Get <%v> pods in <%s> namespace", len(podList), MCO_NAMESPACE)
	for _, pod := range podList {
		isReady := false
		if pod.Status.Phase == corev1.PodRunning {
			isReady = true
			break
		}

		// only print not ready pod status
		if !isReady {
			klog.V(1).Infof("Pod <%s> is not <Ready> on <%s> status due to %#v\n", pod.Name, pod.Status.Phase, pod.Status)
		}
	}
}

func PrintMCOObject(opt TestOptions) {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		klog.V(1).Infof("Failed to get mco object")
		return
	}

	spec, _ := json.MarshalIndent(mco.Object["spec"], "", "  ")
	status, _ := json.MarshalIndent(mco.Object["status"], "", "  ")
	klog.V(1).Infof("MCO spec: %+v\n", string(spec))
	klog.V(1).Infof("MCO status: %+v\n", string(status))
}

func PrintManagedClusterOBAObject(opt TestOptions) {
	clientDynamic := GetKubeClientDynamic(opt, false)
	oba, getErr := clientDynamic.Resource(NewMCOAddonGVR()).Namespace(MCO_ADDON_NAMESPACE).Get(context.TODO(), "observability-addon", metav1.GetOptions{})
	if getErr != nil {
		klog.V(1).Infof("Failed to get oba object from managedcluster")
		return
	}

	spec, _ := json.MarshalIndent(oba.Object["spec"], "", "  ")
	status, _ := json.MarshalIndent(oba.Object["status"], "", "  ")
	klog.V(1).Infof("OBA spec: %+v\n", string(spec))
	klog.V(1).Infof("OBA status: %+v\n", string(status))
}

func GetAllOBAPods(opt TestOptions) ([]corev1.Pod, error) {
	clientKube := getKubeClient(opt, false)
	obaPods, err := clientKube.CoreV1().Pods(MCO_ADDON_NAMESPACE).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return []corev1.Pod{}, err
	}

	return obaPods.Items, nil
}

func PrintAllOBAPodsStatus(opt TestOptions) {
	podList, err := GetAllOBAPods(opt)
	if err != nil {
		klog.Errorf("Failed to get all OBA pods")
	}

	if len(podList) == 0 {
		klog.V(1).Infof("Failed to get pod in <%s> namespace from managedcluster", MCO_ADDON_NAMESPACE)
	}

	klog.V(1).Infof("Get <%v> pods in <%s> namespace from managedcluster", len(podList), MCO_ADDON_NAMESPACE)

	for _, pod := range podList {
		isReady := false
		if pod.Status.Phase == corev1.PodRunning {
			isReady = true
			break
		}

		// only print not ready pod status
		if !isReady {
			klog.V(1).Infof("Pod <%s> is not <Ready> on <%s> status due to %#v\n", pod.Name, pod.Status.Phase, pod.Status)
		}
	}
}

func CheckAllPodNodeSelector(opt TestOptions, nodeSelector map[string]interface{}) error {
	podList, err := GetAllMCOPods(opt)
	if err != nil {
		return err
	}

	for k, v := range nodeSelector {
		for _, pod := range podList {
			selecterValue, ok := pod.Spec.NodeSelector[k]
			if !ok || selecterValue != v {
				return fmt.Errorf("failed to check node selector with %s=%s for pod: %v", k, v, pod.GetName())
			}
		}
	}

	return nil
}

func CheckAllPodsAffinity(opt TestOptions) error {
	podList, err := GetAllMCOPods(opt)
	if err != nil {
		return err
	}

	for _, pod := range podList {

		if pod.Spec.Affinity == nil {
			return fmt.Errorf("Failed to check affinity for pod: %v" + pod.GetName())
		}

		weightedPodAffinityTerms := pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		for _, weightedPodAffinityTerm := range weightedPodAffinityTerms {
			topologyKey := weightedPodAffinityTerm.PodAffinityTerm.TopologyKey
			if (topologyKey == "kubernetes.io/hostname" && weightedPodAffinityTerm.Weight == 30) ||
				(topologyKey == "topology.kubernetes.io/zone" && weightedPodAffinityTerm.Weight == 70) {
			} else {
				return fmt.Errorf("failed to check affinity for pod: %v" + pod.GetName())
			}
		}
	}
	return nil
}

func CheckStorageResize(opt TestOptions, stsName string, expectedCapacity string) error {
	client := getKubeClient(opt, true)
	statefulsets := client.AppsV1().StatefulSets(MCO_NAMESPACE)
	statefulset, err := statefulsets.Get(context.TODO(), stsName, metav1.GetOptions{})
	if err != nil {
		klog.V(1).Infof("Error while retrieving statefulset %s: %s", stsName, err.Error())
		return err
	}
	vct := statefulset.Spec.VolumeClaimTemplates[0]
	if !vct.Spec.Resources.Requests["storage"].Equal(resource.MustParse(expectedCapacity)) {
		err = fmt.Errorf("the storage size of statefulset %s should have %s but got %v",
			stsName, expectedCapacity,
			vct.Spec.Resources.Requests["storage"])
		return err
	}
	return nil
}

func CheckOBAComponents(opt TestOptions) error {
	client := getKubeClient(opt, false)
	deployments := client.AppsV1().Deployments(MCO_ADDON_NAMESPACE)
	expectedDeploymentNames := []string{
		"endpoint-observability-operator",
		"metrics-collector-deployment",
	}

	for _, deploymentName := range expectedDeploymentNames {
		deployment, err := deployments.Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Error while retrieving deployment %s: %s", deploymentName, err.Error())
			return err
		}

		if deployment.Status.ReadyReplicas != *deployment.Spec.Replicas {
			err = fmt.Errorf("deployment %s should have %d but got %d ready replicas",
				deploymentName,
				*deployment.Spec.Replicas,
				deployment.Status.ReadyReplicas)
			return err
		}
	}

	return nil
}

func CheckMCOComponents(opt TestOptions) error {
	client := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	deployments := client.AppsV1().Deployments(MCO_NAMESPACE)
	expectedDeploymentLabels := []string{
		"app=multicluster-observability-grafana",
		"app.kubernetes.io/name=observatorium-api",
		"app.kubernetes.io/name=thanos-query",
		"app.kubernetes.io/name=thanos-query-frontend",
		"app.kubernetes.io/name=thanos-receive-controller",
		"app.kubernetes.io/name=observatorium-operator",
		"app=rbac-query-proxy",
	}

	for _, deploymentLabel := range expectedDeploymentLabels {
		deployList, err := deployments.List(context.TODO(), metav1.ListOptions{
			LabelSelector: deploymentLabel,
		})

		if err != nil {
			klog.Errorf("Error while listing deployment with label %s due to: %s", deploymentLabel, err.Error())
			return err
		}

		for _, deployInfo := range (*deployList).Items {
			if deployInfo.Status.ReadyReplicas != *deployInfo.Spec.Replicas {
				err = fmt.Errorf("deployment %s should have %d but got %d ready replicas",
					deployInfo.Name,
					*deployInfo.Spec.Replicas,
					deployInfo.Status.ReadyReplicas)
				return err
			}
		}
	}

	statefulsets := client.AppsV1().StatefulSets(MCO_NAMESPACE)
	expectedStatefulsetLabels := []string{
		"app=multicluster-observability-alertmanager",
		"app.kubernetes.io/name=thanos-compact",
		"app.kubernetes.io/name=thanos-receive",
		"app.kubernetes.io/name=thanos-rule",
		"app.kubernetes.io/name=memcached",
		"app.kubernetes.io/name=thanos-store",
	}

	for _, statefulsetLabel := range expectedStatefulsetLabels {
		statefulsetList, err := statefulsets.List(context.TODO(), metav1.ListOptions{
			LabelSelector: statefulsetLabel,
		})
		if err != nil {
			klog.V(1).Infof("Error while listing deployment with label %s due to: %s", statefulsetLabel, err.Error())
			return err
		}

		for _, statefulsetInfo := range (*statefulsetList).Items {
			if statefulsetInfo.Status.ReadyReplicas != *statefulsetInfo.Spec.Replicas {
				err = fmt.Errorf("statefulset %s should have %d but got %d ready replicas",
					statefulsetInfo.Name,
					*statefulsetInfo.Spec.Replicas,
					statefulsetInfo.Status.ReadyReplicas)
				return err
			}
		}
	}

	return nil
}

func CheckStatefulSetPodReady(opt TestOptions, stsName string) error {
	client := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	statefulsets := client.AppsV1().StatefulSets(MCO_NAMESPACE)
	statefulset, err := statefulsets.Get(context.TODO(), stsName, metav1.GetOptions{})
	if err != nil {
		klog.V(1).Infof("Error while retrieving statefulset %s: %s", stsName, err.Error())
		return err
	}

	if statefulset.Status.ReadyReplicas != *statefulset.Spec.Replicas ||
		statefulset.Status.UpdatedReplicas != *statefulset.Spec.Replicas ||
		statefulset.Status.UpdateRevision != statefulset.Status.CurrentRevision {
		err = fmt.Errorf("statefulset %s should have %d but got %d ready replicas",
			stsName, *statefulset.Spec.Replicas,
			statefulset.Status.ReadyReplicas)
		return err
	}
	return nil
}

func CheckDeploymentPodReady(opt TestOptions, deployName string) error {
	client := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	deploys := client.AppsV1().Deployments(MCO_NAMESPACE)
	deploy, err := deploys.Get(context.TODO(), deployName, metav1.GetOptions{})
	if err != nil {
		klog.V(1).Infof("Error while retrieving deployment %s: %s", deployName, err.Error())
		return err
	}

	if deploy.Status.ReadyReplicas != *deploy.Spec.Replicas ||
		deploy.Status.UpdatedReplicas != *deploy.Spec.Replicas ||
		deploy.Status.AvailableReplicas != *deploy.Spec.Replicas {
		err = fmt.Errorf("deployment %s should have %d but got %d ready replicas",
			deployName, *deploy.Spec.Replicas,
			deploy.Status.ReadyReplicas)
		return err
	}
	return nil
}

// ModifyMCOCR modifies the MCO CR for reconciling. modify multiple parameter to save running time
func ModifyMCOCR(opt TestOptions) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}
	spec := mco.Object["spec"].(map[string]interface{})
	storageConfig := spec["storageConfig"].(map[string]interface{})
	storageConfig["alertmanagerStorageSize"] = "2Gi"

	advRetentionCon, _ := CheckAdvRetentionConfig(opt)
	if advRetentionCon {
		retentionConfig := spec["advanced"].(map[string]interface{})["retentionConfig"].(map[string]interface{})
		retentionConfig["retentionResolutionRaw"] = "3d"
	}

	_, updateErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Update(context.TODO(), mco, metav1.UpdateOptions{})
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func CheckAdvRetentionConfig(opt TestOptions) (bool, error) {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return false, getErr
	}

	spec := mco.Object["spec"].(map[string]interface{})
	if _, adv := spec["advanced"]; !adv {
		return false, fmt.Errorf("the MCO CR did not have advanced spec configed")
	} else {
		advanced := spec["advanced"].(map[string]interface{})
		if _, rec := advanced["retentionConfig"]; !rec {
			return false, fmt.Errorf("the MCO CR did not have advanced retentionConfig spec configed")
		} else {
			return true, nil
		}
	}
}

// RevertMCOCRModification revert the previous changes
func RevertMCOCRModification(opt TestOptions) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}
	spec := mco.Object["spec"].(map[string]interface{})
	advRetentionCon, _ := CheckAdvRetentionConfig(opt)
	if advRetentionCon {
		retentionConfig := spec["advanced"].(map[string]interface{})["retentionConfig"].(map[string]interface{})
		retentionConfig["retentionResolutionRaw"] = "5d"
	}
	_, updateErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Update(context.TODO(), mco, metav1.UpdateOptions{})
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func CheckMCOAddon(opt TestOptions) error {
	client := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	if len(opt.ManagedClusters) > 0 {
		client = NewKubeClient(
			opt.ManagedClusters[0].MasterURL,
			opt.ManagedClusters[0].KubeConfig,
			"")
	}
	expectedPodNames := []string{
		"endpoint-observability-operator",
		"metrics-collector-deployment",
	}
	podList, err := client.CoreV1().Pods(MCO_ADDON_NAMESPACE).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	podsn := make(map[string]corev1.PodPhase)
	for _, pod := range podList.Items {
		podsn[pod.Name] = pod.Status.Phase
	}
	for _, podName := range expectedPodNames {
		exist := false
		for key, value := range podsn {
			if strings.HasPrefix(key, podName) && value == "Running" {
				exist = true
			}
		}
		if !exist {
			return fmt.Errorf(podName + " not found")
		}
	}
	return nil
}

func CheckMCOAddonResources(opt TestOptions) error {
	client := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	if len(opt.ManagedClusters) > 0 {
		client = NewKubeClient(
			opt.ManagedClusters[0].MasterURL,
			opt.ManagedClusters[0].KubeConfig,
			"")
	}

	deployList, err := client.AppsV1().Deployments(MCO_ADDON_NAMESPACE).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	resMap := make(map[string]corev1.ResourceRequirements)
	for _, deploy := range deployList.Items {
		resMap[deploy.Name] = deploy.Spec.Template.Spec.Containers[0].Resources
	}

	metricsCollectorRes := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			"cpu":    resource.MustParse("200m"),
			"memory": resource.MustParse("700Mi"),
		},
		Requests: corev1.ResourceList{
			"cpu":    resource.MustParse("200m"),
			"memory": resource.MustParse("200Mi"),
		},
	}

	if !reflect.DeepEqual(resMap["metrics-collector-deployment"], metricsCollectorRes) {
		return fmt.Errorf("metrics-collector-deployment resource <%v> is not equal <%v>",
			resMap["metrics-collector-deployment"],
			metricsCollectorRes)
	}

	return nil
}

func ModifyMCORetentionResolutionRaw(opt TestOptions) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}

	spec := mco.Object["spec"].(map[string]interface{})
	advRetentionCon, _ := CheckAdvRetentionConfig(opt)
	if advRetentionCon {
		retentionConfig := spec["advanced"].(map[string]interface{})["retentionConfig"].(map[string]interface{})
		retentionConfig["retentionResolutionRaw"] = "3d"
	}
	_, updateErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Update(context.TODO(), mco, metav1.UpdateOptions{})
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func GetMCOAddonSpecMetrics(opt TestOptions) (bool, error) {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return false, getErr
	}

	enable := mco.Object["spec"].(map[string]interface{})["observabilityAddonSpec"].(map[string]interface{})["enableMetrics"].(bool)
	return enable, nil
}

func ModifyMCOAddonSpecMetrics(opt TestOptions, enable bool) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}

	observabilityAddonSpec := mco.Object["spec"].(map[string]interface{})["observabilityAddonSpec"].(map[string]interface{})
	observabilityAddonSpec["enableMetrics"] = enable
	_, updateErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Update(context.TODO(), mco, metav1.UpdateOptions{})
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func ModifyMCOAddonSpecInterval(opt TestOptions, interval int64) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}

	observabilityAddonSpec := mco.Object["spec"].(map[string]interface{})["observabilityAddonSpec"].(map[string]interface{})
	observabilityAddonSpec["interval"] = interval
	_, updateErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Update(context.TODO(), mco, metav1.UpdateOptions{})
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func GetMCOAddonSpecResources(opt TestOptions) (map[string]interface{}, error) {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	mco, getErr := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if getErr != nil {
		return nil, getErr
	}

	res := mco.Object["spec"].(map[string]interface{})["observabilityAddonSpec"].(map[string]interface{})["resources"].(map[string]interface{})
	return res, nil
}

func DeleteMCOInstance(opt TestOptions) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	return clientDynamic.Resource(NewMCOGVRV1BETA2()).Delete(context.TODO(), MCO_CR_NAME, metav1.DeleteOptions{})
}

func CheckMCOConversion(opt TestOptions, v1beta1tov1beta2GoldenPath string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	getMCO, err := clientDynamic.Resource(NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if err != nil {
		return err
	}

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	yamlB, err := ioutil.ReadFile(v1beta1tov1beta2GoldenPath)
	if err != nil {
		return err
	}

	expectedMCO := &unstructured.Unstructured{}
	_, _, err = decUnstructured.Decode(yamlB, nil, expectedMCO)
	if err != nil {
		return err
	}

	getMCOSpec := getMCO.Object["spec"].(map[string]interface{})
	expectedMCOSpec := expectedMCO.Object["spec"].(map[string]interface{})

	for k, v := range expectedMCOSpec {
		val, ok := getMCOSpec[k]
		if !ok {
			return fmt.Errorf("%s not found in ", k)
		}
		if !reflect.DeepEqual(val, v) {
			return fmt.Errorf("%+v and %+v are not equal", val, v)
		}
	}
	return nil
}

func CreatePullSecret(opt TestOptions, mcoNs string) error {
	clientKube := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	name, err := GetPullSecret(opt)
	if err != nil {
		return err
	}

	pullSecret, errGet := clientKube.CoreV1().Secrets(mcoNs).Get(context.TODO(), name, metav1.GetOptions{})
	if errGet != nil {
		return errGet
	}

	pullSecret.ObjectMeta = metav1.ObjectMeta{
		Name:      name,
		Namespace: MCO_NAMESPACE,
	}
	klog.V(1).Infof("Create MCO pull secret")
	_, err = clientKube.CoreV1().Secrets(pullSecret.Namespace).Create(context.TODO(), pullSecret, metav1.CreateOptions{})
	return err
}

func CreateMCONamespace(opt TestOptions) error {
	ns := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s`,
		MCO_NAMESPACE)
	klog.V(1).Infof("Create %s namespaces", MCO_NAMESPACE)
	return Apply(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext,
		[]byte(ns))
}

func CreateObjSecret(opt TestOptions) error {

	bucket := os.Getenv("BUCKET")
	if bucket == "" {
		return fmt.Errorf("failed to get s3 BUCKET env")
	}

	region := os.Getenv("REGION")
	if region == "" {
		return fmt.Errorf("failed to get s3 REGION env")
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	if accessKey == "" {
		return fmt.Errorf("failed to get aws AWS_ACCESS_KEY_ID env")
	}

	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if secretKey == "" {
		return fmt.Errorf("failed to get aws AWS_SECRET_ACCESS_KEY env")
	}

	objSecret := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
stringData:
  thanos.yaml: |
    type: s3
    config:
      bucket: %s
      endpoint: s3.%s.amazonaws.com
      insecure: false
      access_key: %s
      secret_key: %s
type: Opaque`,
		OBJ_SECRET_NAME,
		MCO_NAMESPACE,
		bucket,
		region,
		accessKey,
		secretKey)
	klog.V(1).Infof("Create MCO object storage secret")
	return Apply(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext,
		[]byte(objSecret))
}

func UninstallMCO(opt TestOptions) error {
	klog.V(1).Infof("Delete MCO instance")
	deleteMCOErr := DeleteMCOInstance(opt)
	if deleteMCOErr != nil {
		return deleteMCOErr
	}

	clientKube := NewKubeClient(
		opt.HubCluster.MasterURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	klog.V(1).Infof("Delete MCO object storage secret")
	deleteObjSecretErr := clientKube.CoreV1().Secrets(MCO_NAMESPACE).Delete(context.TODO(), OBJ_SECRET_NAME, metav1.DeleteOptions{})
	if deleteObjSecretErr != nil {
		return deleteObjSecretErr
	}

	return nil
}

func CreateCustomAlertConfigYaml(baseDomain string) []byte {
	global := fmt.Sprintf(`global:
  resolve_timeout: 5m
route:
  receiver: default-receiver
  routes:
    - match:
        alertname: Watchdog
      receiver: default-receiver
  group_by: ['alertname', 'cluster']
  group_wait: 5s
  group_interval: 5s
  repeat_interval: 2m
receivers:
  - name: default-receiver
    slack_configs:
    - api_url: https://hooks.slack.com/services/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
      channel: team-observability-test
      footer: |
        {{ .CommonLabels.cluster }}
      mrkdwn_in:
        - text
      title: '[{{ .Status | toUpper }}] {{ .CommonLabels.alertname }} ({{ .CommonLabels.severity }})'
      text: |-
        {{ range .Alerts }}
          *Alerts:* {{ .Annotations.summary }}
          *Description:* {{ .Annotations.description }}
          *Details:*
          {{ range .Labels.SortedPairs }} • *{{ .Name }}:* {{ .Value }}
          {{ end }}
        {{ end }}
      title_link: https://multicloud-console.apps.%s/grafana/explore?orgId=1&left=["now-1h","now","Observatorium",{"expr":"ALERTS{alertname=\"{{ .CommonLabels.alertname }}\"}","context":"explore"},{"mode":"Metrics"},{"ui":[true,true,true,"none"]}]
`, baseDomain)
	encodedGlobal := b64.StdEncoding.EncodeToString([]byte(global))

	instance := fmt.Sprintf(`kind: Secret
apiVersion: v1
metadata:
  name: alertmanager-config
  namespace: open-cluster-management-observability
data:
  alertmanager.yaml: >-
    %s
`, encodedGlobal)

	return []byte(instance)
}
