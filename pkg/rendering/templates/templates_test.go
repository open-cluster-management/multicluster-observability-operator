// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package templates

import (
	"os"
	"path"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta1"
)

func TestGetCoreTemplates(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(path.Dir(wd))), "manifests")
	os.Setenv(TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(TemplatesPathEnvVar)

	mchcr := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			ImagePullPolicy: "Always",
			ImagePullSecret: "test",
			StorageConfig: &mcov1beta1.StorageConfigObject{
				StatefulSetStorageClass: "gp2",
			},
		},
	}
	_, err = GetTemplateRenderer().GetTemplates(mchcr)

	if err != nil {
		t.Fatalf("failed to render core template %v", err)
	}
}
