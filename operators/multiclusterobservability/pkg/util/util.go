// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("util")

// Remove is used to remove string from a string array
func Remove(list []string, s string) []string {
	result := []string{}
	for _, v := range list {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// Contains is used to check whether a list contains string s
func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// GetAnnotation returns the annotation value for a given key, or an empty string if not set
func GetAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

// GetPVCList get pvc with matched labels
func GetPVCList(c client.Client, matchLabels map[string]string) ([]corev1.PersistentVolumeClaim, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	pvcListOpts := []client.ListOption{
		client.InNamespace(config.GetDefaultNamespace()),
		client.MatchingLabels(matchLabels),
	}

	err := c.List(context.TODO(), pvcList, pvcListOpts...)
	if err != nil {
		return nil, err
	}
	return pvcList.Items, nil
}

// GetStatefulSetList get sts with matched labels
func GetStatefulSetList(c client.Client, matchLabels map[string]string) ([]appsv1.StatefulSet, error) {
	stsList := &appsv1.StatefulSetList{}
	stsListOpts := []client.ListOption{
		client.InNamespace(config.GetDefaultNamespace()),
		client.MatchingLabels(matchLabels),
	}

	err := c.List(context.TODO(), stsList, stsListOpts...)
	if err != nil {
		return nil, err
	}
	return stsList.Items, nil
}

// GeneratePassword returns a base64 encoded securely random bytes.
func GeneratePassword(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), err
}

// ProxyEnvVarsAreSet ...
// OLM handles these environment variables as a unit;
// if at least one of them is set, all three are considered overridden
// and the cluster-wide defaults are not used for the deployments of the subscribed Operator.
// https://docs.openshift.com/container-platform/4.6/operators/admin/olm-configuring-proxy-support.html
func ProxyEnvVarsAreSet() bool {
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("NO_PROXY") != "" {
		return true
	}
	return false
}

func RemoveDuplicates(elements []string) []string {
	// Use map to record duplicates as we find them.
	encountered := map[string]struct{}{}
	result := []string{}

	for _, v := range elements {
		if _, found := encountered[v]; found {
			continue
		}
		encountered[v] = struct{}{}
		result = append(result, v)
	}
	// Return the new slice.
	return result
}
