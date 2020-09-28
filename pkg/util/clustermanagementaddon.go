// Copyright (c) 2020 Red Hat, Inc.
package util

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonv1alpha1 "github.com/open-cluster-management/addon-framework/api/v1alpha1"
)

const (
	ObservabilityController = "observability-controller"
)

type clusterManagementAddOnSpec struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	CRDName     string `json:"crdName"`
}

func CreateClusterManagementAddon(c client.Client) error {
	addOnFound := false
	clusterManagementAddon := &addonv1alpha1.ClusterManagementAddOn{}
	for !addOnFound {
		if err := c.Get(context.TODO(), types.NamespacedName{Name: ObservabilityController},
			clusterManagementAddon); err != nil {
			if errors.IsNotFound(err) {
				clusterManagementAddon := newClusterManagementAddon()
				if err := c.Create(context.TODO(), clusterManagementAddon); err != nil {
					log.Error(err, "Failed to create observability-controller clustermanagementaddon ")
					return err
				}
				log.Info("Created observability-controller clustermanagementaddon")
				return nil
			}
			switch err.(type) {
			case *cache.ErrCacheNotStarted:
				time.Sleep(time.Second)
				continue
			default:
				log.Error(err, "Cannot create observability-controller clustermanagementaddon")
			}
			return err
		}

		log.Info(fmt.Sprintf("%s clustermanagementaddon is present ", ObservabilityController))
		return nil
	}
	return nil
}

func newClusterManagementAddon() *addonv1alpha1.ClusterManagementAddOn {
	clusterManagementAddOnSpec := clusterManagementAddOnSpec{
		DisplayName: "Observability Controller",
		Description: "Manages Observability components.",
		CRDName:     "observabilityaddons.observability.open-cluster-management.io",
	}
	return &addonv1alpha1.ClusterManagementAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterManagementAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
			Annotations: map[string]string{
				"console.open-cluster-management.io/launch-link":      "/grafana/d/gSyJ7YIMk/acm-clusters-overview",
				"console.open-cluster-management.io/launch-link-text": "Grafana",
			},
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: clusterManagementAddOnSpec.DisplayName,
				Description: clusterManagementAddOnSpec.Description,
			},
			AddOnConfiguration: addonv1alpha1.ConfigCoordinates{
				CRDName: clusterManagementAddOnSpec.CRDName,
			},
		},
	}
}