// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
)

func installMCO() {
	if os.Getenv("SKIP_INSTALL_STEP") == "true" {
		return
	}

	hubClient := utils.NewKubeClient(
		testOptions.HubCluster.MasterURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	dynClient := utils.NewKubeClientDynamic(
		testOptions.HubCluster.MasterURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	By("Checking MCO operator is existed")
	podList, err := hubClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: MCO_LABEL})
	Expect(len(podList.Items)).To(Equal(1))
	Expect(err).NotTo(HaveOccurred())
	var (
		mcoPod = ""
		mcoNs  = ""
	)
	for _, pod := range podList.Items {
		mcoPod = pod.GetName()
		mcoNs = pod.GetNamespace()
		Expect(string(mcoPod)).NotTo(Equal(""))
		Expect(string(pod.Status.Phase)).To(Equal("Running"))
	}

	// print mco logs if MCO installation failed
	defer func(testOptions utils.TestOptions, isHub bool, namespace, podName, containerName string, previous bool, tailLines int64) {
		if testFailed {
			mcoLogs, err := utils.GetPodLogs(testOptions, isHub, namespace, podName, containerName, previous, tailLines)
			Expect(err).NotTo(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "[DEBUG] MCO is installed failed, checking MCO operator logs:\n%s\n", mcoLogs)
		} else {
			fmt.Fprintf(GinkgoWriter, "[DEBUG] MCO is installed successfully!\n")
		}
	}(testOptions, false, mcoNs, mcoPod, "multicluster-observability-operator", false, 1000)

	By("Checking Required CRDs is existed")
	Eventually(func() error {
		return utils.HaveCRDs(testOptions.HubCluster, testOptions.KubeConfig,
			[]string{
				"multiclusterobservabilities.observability.open-cluster-management.io",
				"observatoria.core.observatorium.io",
				"observabilityaddons.observability.open-cluster-management.io",
			})
	}).Should(Succeed())

	Expect(utils.CreateMCONamespace(testOptions)).NotTo(HaveOccurred())
	if os.Getenv("IS_CANARY_ENV") == "true" {
		Expect(utils.CreatePullSecret(testOptions, mcoNs)).NotTo(HaveOccurred())
		Expect(utils.CreateObjSecret(testOptions)).NotTo(HaveOccurred())
	}
	//set resource quota and limit range for canary environment to avoid destruct the node
	yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/policy"})
	Expect(err).NotTo(HaveOccurred())
	Expect(utils.Apply(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, yamlB)).NotTo(HaveOccurred())

	if os.Getenv("IS_CANARY_ENV") != "true" {
		By("Creating the MCO testing RBAC resources")
		Expect(utils.CreateMCOTestingRBAC(testOptions)).NotTo(HaveOccurred())
	}

	if os.Getenv("SKIP_INTEGRATION_CASES") != "true" {
		By("Creating MCO instance of v1beta1")
		v1beta1KustomizationPath := "../../../examples/mco/e2e/v1beta1"
		yamlB, err = kustomize.Render(kustomize.Options{KustomizationPath: v1beta1KustomizationPath})
		Expect(err).NotTo(HaveOccurred())
		Expect(utils.Apply(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, yamlB)).NotTo(HaveOccurred())

		By("Waiting for MCO ready status")
		allPodsIsReady := false
		Eventually(func() error {
			instance, err := dynClient.Resource(utils.NewMCOGVRV1BETA1()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
			if err == nil {
				allPodsIsReady = utils.StatusContainsTypeEqualTo(instance, "Ready")
				if allPodsIsReady {
					testFailed = false
					return nil
				}
			}
			testFailed = true
			if instance != nil && instance.Object != nil {
				return fmt.Errorf("MCO componnets cannot be running in 20 minutes. check the MCO CR status for the details: %v", instance.Object["status"])
			} else {
				return fmt.Errorf("Wait for reconciling.")
			}
		}, EventuallyTimeoutMinute*20, EventuallyIntervalSecond*5).Should(Succeed())

		By("Check clustermanagementaddon CR is created")
		Eventually(func() error {
			_, err := dynClient.Resource(utils.NewMCOClusterManagementAddonsGVR()).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
			if err != nil {
				testFailed = true
				return err
			}
			testFailed = false
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

		By("Check the api conversion is working as expected")
		v1beta1Tov1beta2GoldenPath := "../../../examples/mco/e2e/v1beta1/observability-v1beta1-to-v1beta2-golden.yaml"
		err = utils.CheckMCOConversion(testOptions, v1beta1Tov1beta2GoldenPath)
		Expect(err).NotTo(HaveOccurred())
	}

	By("Apply MCO instance of v1beta2")
	v1beta2KustomizationPath := "../../../examples/mco/e2e/v1beta2"
	yamlB, err = kustomize.Render(kustomize.Options{KustomizationPath: v1beta2KustomizationPath})
	Expect(err).NotTo(HaveOccurred())

	// add retry for update mco object failure
	Eventually(func() error {
		return utils.Apply(testOptions.HubCluster.MasterURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, yamlB)
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	// wait for pod restarting
	time.Sleep(60 * time.Second)

	By("Waiting for MCO ready status")
	Eventually(func() error {
		err = utils.CheckMCOComponents(testOptions)
		if err != nil {
			testFailed = true
			utils.PrintAllMCOPodsStatus(testOptions)
			return err
		}
		testFailed = false
		return nil
	}, EventuallyTimeoutMinute*25, EventuallyIntervalSecond*10).Should(Succeed())

	By("Check endpoint-operator and metrics-collector pods are created")
	Eventually(func() error {
		err = utils.CheckMCOAddon(testOptions)
		if err != nil {
			testFailed = true
			return err
		}
		testFailed = false
		return nil
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

	By("Check clustermanagementaddon CR is created")
	Eventually(func() error {
		_, err := dynClient.Resource(utils.NewMCOClusterManagementAddonsGVR()).Get(context.TODO(), "observability-controller", metav1.GetOptions{})
		if err != nil {
			testFailed = true
			return err
		}
		testFailed = false
		return nil
	}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
}
