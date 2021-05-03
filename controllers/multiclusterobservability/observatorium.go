// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"fmt"
	"reflect"

	obsv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

const (
	obsAPIGateway = "observatorium-api"

	readOnlyRoleName  = "read-only-metrics"
	writeOnlyRoleName = "write-only-metrics"
)

// GenerateObservatoriumCR returns Observatorium cr defined in MultiClusterObservability
func GenerateObservatoriumCR(
	cl client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	labels := map[string]string{
		"app": mco.Name,
	}

	storageClassSelected, err := getStorageClass(mco, cl)
	if err != nil {
		return &ctrl.Result{}, err
	}

	log.Info("storageClassSelected", "storageClassSelected", storageClassSelected)

	observatoriumCR := &obsv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mco.Name,
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    labels,
		},
		Spec: *newDefaultObservatoriumSpec(mco, storageClassSelected),
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, observatoriumCR, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &obsv1alpha1.Observatorium{}
	err = cl.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      observatoriumCR.Name,
			Namespace: observatoriumCR.Namespace,
		},
		observatoriumCRFound,
	)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new observatorium CR",
			"observatorium", observatoriumCR,
		)
		err = cl.Create(context.TODO(), observatoriumCR)
		if err != nil {
			return &ctrl.Result{}, err
		}
		return nil, nil
	} else if err != nil {
		return &ctrl.Result{}, err
	}

	oldSpec := observatoriumCRFound.Spec
	newSpec := observatoriumCR.Spec
	// @TODO: resolve design issue on whether enable/disable downsampling will affact retension period config
	if reflect.DeepEqual(newSpec, oldSpec) {
		return nil, nil
	}

	// keep the tenant id unchanged
	for i, newTenant := range newSpec.API.Tenants {
		for _, oldTenant := range oldSpec.API.Tenants {
			updateTenantID(&newSpec, newTenant, oldTenant, i)
		}
	}

	newObj := observatoriumCRFound.DeepCopy()
	newObj.Spec = newSpec
	err = cl.Update(context.TODO(), newObj)
	if err != nil {
		return &ctrl.Result{}, err
	}

	// delete the store-share statefulset in scalein scenario
	err = deleteStoreSts(cl, observatoriumCR.Name,
		*oldSpec.Thanos.Store.Shards, *newSpec.Thanos.Store.Shards)
	if err != nil {
		return &ctrl.Result{}, err
	}

	return nil, nil
}

func updateTenantID(
	newSpec *obsv1alpha1.ObservatoriumSpec,
	newTenant obsv1alpha1.APITenant,
	oldTenant obsv1alpha1.APITenant,
	idx int) {

	if oldTenant.Name == newTenant.Name && newTenant.ID == oldTenant.ID {
		return
	}

	newSpec.API.Tenants[idx].ID = oldTenant.ID
	for j, hashring := range newSpec.Hashrings {
		if util.Contains(hashring.Tenants, newTenant.ID) {
			newSpec.Hashrings[j].Tenants = util.Remove(newSpec.Hashrings[j].Tenants, newTenant.ID)
			newSpec.Hashrings[j].Tenants = append(newSpec.Hashrings[0].Tenants, oldTenant.ID)
		}
	}
}

// GenerateAPIGatewayRoute defines aaa
func GenerateAPIGatewayRoute(
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	apiGateway := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("public"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: mco.Name + "-observatorium-api",
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationPassthrough,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyNone,
			},
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, apiGateway, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	err := runclient.Get(
		context.TODO(),
		types.NamespacedName{Name: apiGateway.Name, Namespace: apiGateway.Namespace},
		&routev1.Route{})
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new route to expose observatorium api",
			"apiGateway.Namespace", apiGateway.Namespace,
			"apiGateway.Name", apiGateway.Name,
		)
		err = runclient.Create(context.TODO(), apiGateway)
		if err != nil {
			return &ctrl.Result{}, err
		}
	}

	return nil, nil
}

func newDefaultObservatoriumSpec(mco *mcov1beta2.MultiClusterObservability,
	scSelected string) *obsv1alpha1.ObservatoriumSpec {

	obs := &obsv1alpha1.ObservatoriumSpec{}
	obs.SecurityContext = &v1.SecurityContext{}
	obs.NodeSelector = mco.Spec.NodeSelector
	obs.Tolerations = mco.Spec.Tolerations
	obs.API = newAPISpec(mco)
	obs.Thanos = newThanosSpec(mco, scSelected)

	obs.Hashrings = []*obsv1alpha1.Hashring{
		{Hashring: "default", Tenants: []string{mcoconfig.GetTenantUID()}},
	}

	obs.ObjectStorageConfig.Thanos = &obsv1alpha1.ThanosObjectStorageConfigSpec{}
	if mco.Spec.StorageConfig != nil && mco.Spec.StorageConfig.MetricObjectStorage != nil {
		objStorageConf := mco.Spec.StorageConfig.MetricObjectStorage
		obs.ObjectStorageConfig.Thanos.Name = objStorageConf.Name
		obs.ObjectStorageConfig.Thanos.Key = objStorageConf.Key
	}
	return obs
}

func newAPIRBAC() obsv1alpha1.APIRBAC {
	return obsv1alpha1.APIRBAC{
		Roles: []obsv1alpha1.RBACRole{
			{
				Name: readOnlyRoleName,
				Resources: []string{
					"metrics",
				},
				Permissions: []obsv1alpha1.Permission{
					obsv1alpha1.Read,
				},
				Tenants: []string{
					mcoconfig.GetDefaultTenantName(),
				},
			},
			{
				Name: writeOnlyRoleName,
				Resources: []string{
					"metrics",
				},
				Permissions: []obsv1alpha1.Permission{
					obsv1alpha1.Write,
				},
				Tenants: []string{
					mcoconfig.GetDefaultTenantName(),
				},
			},
		},
		RoleBindings: []obsv1alpha1.RBACRoleBinding{
			{
				Name: readOnlyRoleName,
				Roles: []string{
					readOnlyRoleName,
				},
				Subjects: []obsv1alpha1.Subject{
					{
						Name: config.GrafanaCN,
						Kind: obsv1alpha1.User,
					},
				},
			},
			{
				Name: writeOnlyRoleName,
				Roles: []string{
					writeOnlyRoleName,
				},
				Subjects: []obsv1alpha1.Subject{
					{
						Name: config.ManagedClusterOU,
						Kind: obsv1alpha1.Group,
					},
				},
			},
		},
	}
}

func newAPITenants() []obsv1alpha1.APITenant {
	return []obsv1alpha1.APITenant{
		{
			Name: mcoconfig.GetDefaultTenantName(),
			ID:   mcoconfig.GetTenantUID(),
			MTLS: &obsv1alpha1.TenantMTLS{
				SecretName: config.ClientCACerts,
				CAKey:      "tls.crt",
			},
		},
	}
}

func newAPITLS() obsv1alpha1.TLS {
	return obsv1alpha1.TLS{
		SecretName: config.ServerCerts,
		CertKey:    "tls.crt",
		KeyKey:     "tls.key",
		CAKey:      "ca.crt",
		ServerName: config.ServerCertCN,
	}
}

func newAPISpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.APISpec {
	apiSpec := obsv1alpha1.APISpec{}
	apiSpec.RBAC = newAPIRBAC()
	apiSpec.Tenants = newAPITenants()
	apiSpec.TLS = newAPITLS()
	apiSpec.Replicas = mcoconfig.GetObservabilityComponentReplicas(mcoconfig.ObservatoriumAPI)
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		apiSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ObservatoriumAPICPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ObservatoriumAPIMemoryRequets),
			},
		}
	}
	//set the default observatorium components' image
	apiSpec.Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.ObservatoriumAPIImgName +
		":" + mcoconfig.ObservatoriumAPIImgTag
	apiSpec.Version = mcoconfig.ObservatoriumAPIImgTag
	replace, image := mcoconfig.ReplaceImage(mco.Annotations, apiSpec.Image, mcoconfig.ObservatoriumAPIImgName)
	if replace {
		apiSpec.Image = image
	}
	apiSpec.ServiceMonitor = true
	return apiSpec
}

func newReceiversSpec(
	mco *mcov1beta2.MultiClusterObservability,
	scSelected string) obsv1alpha1.ReceiversSpec {
	receSpec := obsv1alpha1.ReceiversSpec{}
	receSpec.Retention = mco.Spec.RetentionConfig.RetentionInLocal
	receSpec.Replicas = mcoconfig.GetObservabilityComponentReplicas(mcoconfig.ThanosReceive)
	if *receSpec.Replicas < 3 {
		receSpec.ReplicationFactor = receSpec.Replicas
	} else {
		receSpec.ReplicationFactor = &config.Replicas3
	}

	receSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		receSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosReceiveCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosReceiveMemoryRequets),
			},
		}
	}
	receSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.ReceiveStorageSize,
		scSelected)

	return receSpec
}

func newRuleSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.RuleSpec {
	ruleSpec := obsv1alpha1.RuleSpec{}
	ruleSpec.BlockDuration = mco.Spec.RetentionConfig.BlockDuration
	ruleSpec.Retention = mco.Spec.RetentionConfig.RetentionInLocal
	ruleSpec.Replicas = mcoconfig.GetObservabilityComponentReplicas(mcoconfig.ThanosRule)
	ruleSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		ruleSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosRuleCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosRuleMemoryRequets),
			},
		}
		ruleSpec.ReloaderResources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosRuleReloaderCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosRuleReloaderMemoryRequets),
			},
		}
	}

	ruleSpec.ReloaderImage = mcoconfig.ConfigmapReloaderImgRepo + "/" +
		mcoconfig.ConfigmapReloaderImgName + ":" + mcoconfig.ConfigmapReloaderImgTagSuffix
	found, reloaderImage := mcoconfig.ReplaceImage(mco.Annotations,
		mcoconfig.ConfigmapReloaderImgRepo, mcoconfig.ConfigmapReloaderKey)
	if found {
		ruleSpec.ReloaderImage = reloaderImage
	}

	ruleSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.RuleStorageSize,
		scSelected)

	//configure alertmanager in ruler
	ruleSpec.AlertmanagerURLs = []string{mcoconfig.AlertmanagerURL}
	ruleSpec.RulesConfig = []obsv1alpha1.RuleConfig{
		{
			Name: mcoconfig.AlertRuleDefaultConfigMapName,
			Key:  mcoconfig.AlertRuleDefaultFileKey,
		},
	}

	if mcoconfig.HasCustomRuleConfigMap() {
		customRuleConfig := []obsv1alpha1.RuleConfig{
			{
				Name: mcoconfig.AlertRuleCustomConfigMapName,
				Key:  mcoconfig.AlertRuleCustomFileKey,
			},
		}
		ruleSpec.RulesConfig = append(ruleSpec.RulesConfig, customRuleConfig...)
	} else {
		ruleSpec.RulesConfig = []obsv1alpha1.RuleConfig{
			{
				Name: mcoconfig.AlertRuleDefaultConfigMapName,
				Key:  mcoconfig.AlertRuleDefaultFileKey,
			},
		}
	}

	return ruleSpec
}

func newStoreSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.StoreSpec {
	storeSpec := obsv1alpha1.StoreSpec{}
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		storeSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosStoreCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosStoreMemoryRequets),
			},
		}
	}

	storeSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StoreStorageSize,
		scSelected)
	storeSpec.Shards = &mcoconfig.Replicas3
	storeSpec.ServiceMonitor = true
	storeSpec.Cache = newMemCacheSpec(mco)

	return storeSpec
}

func newMemCacheSpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.MemCacheSpec {
	memeCacheSpec := obsv1alpha1.MemCacheSpec{}
	memeCacheSpec.Image = mcoconfig.MemcachedImgRepo + "/" +
		mcoconfig.MemcachedImgName + ":" + mcoconfig.MemcachedImgTag
	memeCacheSpec.Version = mcoconfig.MemcachedImgTag
	memeCacheSpec.Replicas = mcoconfig.GetObservabilityComponentReplicas(mcoconfig.ThanosStoreMemcached)
	memeCacheSpec.ServiceMonitor = true
	memeCacheSpec.ExporterImage = mcoconfig.MemcachedExporterImgRepo + "/" +
		mcoconfig.MemcachedExporterImgName + ":" + mcoconfig.MemcachedExporterImgTag
	memeCacheSpec.ExporterVersion = mcoconfig.MemcachedExporterImgTag
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		memeCacheSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosCahcedCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosCahcedMemoryRequets),
			},
		}
		memeCacheSpec.ExporterResources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosCahcedExporterCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosCahcedExporterMemoryRequets),
			},
		}
	}

	found, image := mcoconfig.ReplaceImage(mco.Annotations, memeCacheSpec.Image, mcoconfig.MemcachedImgName)
	if found {
		memeCacheSpec.Image = image
	}

	found, image = mcoconfig.ReplaceImage(mco.Annotations, memeCacheSpec.ExporterImage, mcoconfig.MemcachedExporterKey)
	if found {
		memeCacheSpec.ExporterImage = image
	}

	limit := int32(1024)
	memeCacheSpec.MemoryLimitMB = &limit

	return memeCacheSpec
}

func newThanosSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.ThanosSpec {
	thanosSpec := obsv1alpha1.ThanosSpec{}
	thanosSpec.Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.ThanosImgName + ":" + mcoconfig.ThanosImgTag
	thanosSpec.Version = mcoconfig.ThanosImgTag

	thanosSpec.Compact = newCompactSpec(mco, scSelected)
	thanosSpec.Receivers = newReceiversSpec(mco, scSelected)
	thanosSpec.Rule = newRuleSpec(mco, scSelected)
	thanosSpec.Store = newStoreSpec(mco, scSelected)
	thanosSpec.ReceiveController = newReceiverControllerSpec(mco)
	thanosSpec.Query = newQuerySpec(mco)
	thanosSpec.QueryFrontend = newQueryFrontendSpec(mco)

	replace, image := mcoconfig.ReplaceImage(mco.Annotations, thanosSpec.Image, mcoconfig.ThanosImgName)
	if replace {
		thanosSpec.Image = image
	}
	thanosSpec.Version = "2.3.0-SNAPSHOT-2021-04-27-14-40-48"
	thanosSpec.Image = "quay.io/open-cluster-management/thanos:2.3.0-SNAPSHOT-2021-04-27-14-40-48"
	return thanosSpec
}

func newQueryFrontendSpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.QueryFrontendSpec {
	queryFrontendSpec := obsv1alpha1.QueryFrontendSpec{}
	queryFrontendSpec.Replicas = mcoconfig.GetObservabilityComponentReplicas(mcoconfig.ThanosQueryFrontend)
	queryFrontendSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		queryFrontendSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosQueryFrontendCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosQueryFrontendMemoryRequets),
			},
		}
	}
	queryFrontendSpec.Cache = newMemCacheSpec(mco)
	return queryFrontendSpec
}

func newQuerySpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.QuerySpec {
	querySpec := obsv1alpha1.QuerySpec{}
	querySpec.Replicas = mcoconfig.GetObservabilityComponentReplicas(mcoconfig.ThanosQuery)
	querySpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		querySpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosQueryCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosQueryMemoryRequets),
			},
		}
	}
	return querySpec
}

func newReceiverControllerSpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.ReceiveControllerSpec {
	receiveControllerSpec := obsv1alpha1.ReceiveControllerSpec{}
	receiveControllerSpec.Image = mcoconfig.ObservatoriumImgRepo + "/" +
		mcoconfig.ThanosReceiveControllerImgName +
		":" + mcoconfig.ThanosReceiveControllerImgTag
	receiveControllerSpec.ServiceMonitor = true
	receiveControllerSpec.Version = mcoconfig.ThanosReceiveControllerImgTag
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		receiveControllerSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ObservatoriumReceiveControllerCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ObservatoriumReceiveControllerMemoryRequets),
			},
		}
	}
	replace, image := mcoconfig.ReplaceImage(mco.Annotations, receiveControllerSpec.Image,
		mcoconfig.ThanosReceiveControllerKey)
	if replace {
		receiveControllerSpec.Image = image
	}
	return receiveControllerSpec
}

func newCompactSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.CompactSpec {
	compactSpec := obsv1alpha1.CompactSpec{}
	//Compactor, generally, does not need to be highly available.
	//Compactions are needed from time to time, only when new blocks appear.
	compactSpec.Replicas = &mcoconfig.Replicas1
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		compactSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosCompactCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosCompactMemoryRequets),
			},
		}
	}
	compactSpec.ServiceMonitor = true
	compactSpec.EnableDownsampling = mco.Spec.EnableDownsampling
	compactSpec.DeleteDelay = mco.Spec.RetentionConfig.DeleteDelay
	compactSpec.RetentionResolutionRaw = mco.Spec.RetentionConfig.RetentionResolutionRaw
	compactSpec.RetentionResolution5m = mco.Spec.RetentionConfig.RetentionResolution5m
	compactSpec.RetentionResolution1h = mco.Spec.RetentionConfig.RetentionResolution1h
	compactSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.CompactStorageSize,
		scSelected)

	return compactSpec
}

func newVolumeClaimTemplate(size string, storageClass string) obsv1alpha1.VolumeClaimTemplate {
	vct := obsv1alpha1.VolumeClaimTemplate{}
	vct.Spec = v1.PersistentVolumeClaimSpec{
		AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		StorageClassName: &storageClass,
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse(size),
			},
		},
	}
	return vct
}

func mergeVolumeClaimTemplate(oldVolumn,
	newVolumn obsv1alpha1.VolumeClaimTemplate) obsv1alpha1.VolumeClaimTemplate {
	requestRes := newVolumn.Spec.Resources.Requests
	limitRes := newVolumn.Spec.Resources.Limits
	if requestRes != nil {
		oldVolumn.Spec.Resources.Requests[v1.ResourceStorage] = requestRes[v1.ResourceStorage]
	}
	if limitRes != nil {
		oldVolumn.Spec.Resources.Limits[v1.ResourceStorage] = limitRes[v1.ResourceStorage]
	}
	return oldVolumn
}

func deleteStoreSts(cl client.Client, name string, oldNum int32, newNum int32) error {
	if oldNum > newNum {
		for i := newNum; i < oldNum; i++ {
			stsName := fmt.Sprintf("%s-thanos-store-shard-%d", name, i)
			found := &appsv1.StatefulSet{}
			err := cl.Get(context.TODO(), types.NamespacedName{Name: stsName, Namespace: mcoconfig.GetDefaultNamespace()}, found)
			if err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to get statefulset", "name", stsName)
					return err
				}
			} else {
				err = cl.Delete(context.TODO(), found)
				if err != nil {
					log.Error(err, "Failed to delete statefulset", "name", stsName)
					return err
				}
			}
		}
	}
	return nil
}
