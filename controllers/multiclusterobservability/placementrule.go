// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	appsv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

func createPlacementRule(client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) error {
	name := config.GetDefaultCRName()
	namespace := config.GetDefaultNamespace()
	p := &appsv1.PlacementRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.PlacementRuleSpec{
			GenericPlacementFields: appsv1.GenericPlacementFields{
				ClusterSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "observability",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"disabled"},
						},
						{
							Key:      "vendor",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"OpenShift"},
						},
						{
							Key:      "openshiftVersion",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"3"},
						},
					},
				},
			},
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if namespace == config.GetDefaultNamespace() {
		if err := controllerutil.SetControllerReference(mco, p, scheme); err != nil {
			return err
		}
	}

	found := &appsv1.PlacementRule{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating PlacementRule", "name", name)
		err = client.Create(context.TODO(), p)
		if err != nil {
			log.Error(err, "Failed to create PlacementRule", "name", name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check PlacementRule", "name", name)
		return err
	}

	if !reflect.DeepEqual(found.Spec, p.Spec) {
		log.Info("Reverting PlacementRule", "name", name)
		p.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), p)
		if err != nil {
			log.Error(err, "Failed to revert PlacementRule", "name", name)
			return err
		}
		return nil
	}

	log.Info("PlacementRule already existed", "name", name)
	return nil
}
