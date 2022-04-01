/*
This file is part of Cloud Native PostgreSQL.

Copyright (C) 2019-2022 EnterpriseDB Corporation.
*/

package e2e

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/EnterpriseDB/cloud-native-postgresql/pkg/specs/pgbouncer"
	"github.com/EnterpriseDB/cloud-native-postgresql/tests"
	"github.com/EnterpriseDB/cloud-native-postgresql/tests/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PGBouncer Types", Ordered, func() {
	const (
		sampleFile                    = fixturesDir + "/pgbouncer/cluster-pgbouncer.yaml"
		poolerCertificateRWSampleFile = fixturesDir + "/pgbouncer/pgbouncer_types/pgbouncer-pooler-rw.yaml"
		poolerCertificateROSampleFile = fixturesDir + "/pgbouncer/pgbouncer_types/pgbouncer-pooler-ro.yaml"
		level                         = tests.Low
		poolerResourceNameRW          = "pooler-connection-rw"
		poolerResourceNameRO          = "pooler-connection-ro"
		poolerServiceRW               = "cluster-pgbouncer-rw"
		poolerServiceRO               = "cluster-pgbouncer-ro"
	)

	var namespace, clusterName string

	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
	})

	JustAfterEach(func() {
		if CurrentSpecReport().Failed() {
			env.DumpClusterEnv(namespace, clusterName,
				"out/"+CurrentSpecReport().LeafNodeText+".log")
		}
	})

	BeforeAll(func() {
		// Create a cluster in a namespace we'll delete after the test
		// This cluster will be shared by the next tests
		namespace = "pgbouncer-types"
		err := env.CreateNamespace(namespace)
		Expect(err).ToNot(HaveOccurred())
		clusterName, err = env.GetResourceNameFromYAML(sampleFile)
		Expect(err).ToNot(HaveOccurred())
		AssertCreateCluster(namespace, clusterName, sampleFile, env)
	})
	AfterAll(func() {
		err := env.DeleteNamespace(namespace)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should have proper service ip and host details for ro and rw with default installation", func() {
		By("setting up read write type pgbouncer pooler", func() {
			createAndAssertPgBouncerPoolerIsSetUp(namespace, poolerCertificateRWSampleFile, 2)
		})

		By("setting up read only type pgbouncer pooler", func() {
			createAndAssertPgBouncerPoolerIsSetUp(namespace, poolerCertificateROSampleFile, 2)
		})

		By("verify that read-only pooler endpoints contain the correct pod addresses", func() {
			assertPGBouncerEndpointsContainsPodsIP(namespace, poolerCertificateROSampleFile, 2)
		})

		By("verify that read-only pooler pgbouncer.ini contains the correct host service", func() {
			poolerName, err := env.GetResourceNameFromYAML(poolerCertificateROSampleFile)
			Expect(err).ToNot(HaveOccurred())
			podList := &corev1.PodList{}
			err = env.Client.List(env.Ctx, podList, ctrlclient.InNamespace(namespace),
				ctrlclient.MatchingLabels{pgbouncer.PgbouncerNameLabel: poolerName})
			Expect(err).ToNot(HaveOccurred())

			assertPGBouncerHasServiceNameInsideHostParameter(namespace, poolerServiceRO, podList)
		})

		By("verify that read-write pooler endpoints contain the correct pod addresses.", func() {
			assertPGBouncerEndpointsContainsPodsIP(namespace, poolerCertificateRWSampleFile, 2)
		})

		By("verify that read-write pooler pgbouncer.ini contains the correct host service", func() {
			poolerName, err := env.GetResourceNameFromYAML(poolerCertificateRWSampleFile)
			Expect(err).ToNot(HaveOccurred())
			podList := &corev1.PodList{}
			err = env.Client.List(env.Ctx, podList, ctrlclient.InNamespace(namespace),
				ctrlclient.MatchingLabels{pgbouncer.PgbouncerNameLabel: poolerName})
			Expect(err).ToNot(HaveOccurred())

			assertPGBouncerHasServiceNameInsideHostParameter(namespace, poolerServiceRW, podList)
		})
	})

	scalingTest := func(instances int) func() {
		return func() {
			By(fmt.Sprintf("scaling PGBouncer to %v instances", instances), func() {
				command := fmt.Sprintf("kubectl scale pooler %s -n %s --replicas=%v",
					poolerResourceNameRO, namespace, instances)
				_, _, err := utils.Run(command)
				Expect(err).ToNot(HaveOccurred())

				// verifying if PGBouncer pooler pods are ready after scale up
				assertPGBouncerPodsAreReady(namespace, poolerCertificateROSampleFile, instances)

				// // scale up command for 3 replicas for read write
				command = fmt.Sprintf("kubectl scale pooler %s -n %s --replicas=%v",
					poolerResourceNameRW, namespace, instances)
				_, _, err = utils.Run(command)
				Expect(err).ToNot(HaveOccurred())

				// verifying if PGBouncer pooler pods are ready after scale up
				assertPGBouncerPodsAreReady(namespace, poolerCertificateRWSampleFile, instances)
			})

			By("verifying that read-only pooler endpoints contain the correct pod addresses", func() {
				assertPGBouncerEndpointsContainsPodsIP(namespace, poolerCertificateROSampleFile, instances)
			})

			By("verifying that read-only pooler pgbouncer.ini contains the correct host service", func() {
				poolerName, err := env.GetResourceNameFromYAML(poolerCertificateROSampleFile)
				Expect(err).ToNot(HaveOccurred())
				podList := &corev1.PodList{}
				err = env.Client.List(env.Ctx, podList, ctrlclient.InNamespace(namespace),
					ctrlclient.MatchingLabels{pgbouncer.PgbouncerNameLabel: poolerName})
				Expect(err).ToNot(HaveOccurred())

				assertPGBouncerHasServiceNameInsideHostParameter(namespace, poolerServiceRO, podList)
			})

			By("verifying that read-write pooler endpoints contain the correct pod addresses.", func() {
				assertPGBouncerEndpointsContainsPodsIP(namespace, poolerCertificateRWSampleFile, instances)
			})

			By("verifying that read-write pooler pgbouncer.ini contains the correct host service", func() {
				poolerName, err := env.GetResourceNameFromYAML(poolerCertificateRWSampleFile)
				Expect(err).ToNot(HaveOccurred())
				podList := &corev1.PodList{}
				err = env.Client.List(env.Ctx, podList, ctrlclient.InNamespace(namespace),
					ctrlclient.MatchingLabels{pgbouncer.PgbouncerNameLabel: poolerName})
				Expect(err).ToNot(HaveOccurred())
				assertPGBouncerHasServiceNameInsideHostParameter(namespace, poolerServiceRW, podList)
			})
		}
	}

	It("has proper service ip and host details for ro and rw scaling up", scalingTest(3))
	It("has proper service ip and host details for ro and rw scaling down", scalingTest(1))
})
