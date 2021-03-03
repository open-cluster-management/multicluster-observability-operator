// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"testing"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEndpointConfigCR(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newTestRoute(), newTestInfra()}
	c := fake.NewFakeClient(objs...)

	err := createObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to create observabilityaddon: (%v)", err)
	}
	found := &mcov1beta1.ObservabilityAddon{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to get observabilityaddon: (%v)", err)
	}

	err = createObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to create observabilityaddon: (%v)", err)
	}

	err = deleteObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}
	err = c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}

	err = deleteObsAddon(c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete observabilityaddon: (%v)", err)
	}

}
