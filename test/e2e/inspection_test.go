package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var _ = Describe("Inspection", func() {
	var (
		specName       = "inspection"
		namespace      *corev1.Namespace
		cancelWatches  context.CancelFunc
		bmcUser        string
		bmcPassword    string
		bmcAddress     string
		bootMacAddress string
	)
	BeforeEach(func() {
		bmcUser = e2eConfig.GetVariable("BMC_USER")
		bmcPassword = e2eConfig.GetVariable("BMC_PASSWORD")
		bmcAddress = e2eConfig.GetVariable("BMC_ADDRESS")
		bootMacAddress = e2eConfig.GetVariable("BOOT_MAC_ADDRESS")

		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   clusterProxy.GetClient(),
			ClientSet: clusterProxy.GetClientSet(),
			Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			LogFolder: artifactFolder,
		})
	})

	It("should put BMH without BMC credentials in unmanaged state", func() {
		By("creating a BMH")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-unmanaged",
				Namespace: namespace.Name,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the BMH to be in unmanaged state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateUnmanaged,
		}, e2eConfig.GetIntervals(specName, "wait-unmanaged")...)
	})

	It("should fail to register the BMH if the secret is missing", func() {
		By("creating a BMH")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-registration-error",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:         "ipmi://127.0.0.1:5678",
					CredentialsName: "bmc-credentials",
				},
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("trying to register the BMH")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateRegistering,
		}, e2eConfig.GetIntervals(specName, "wait-registering")...)

		By("waiting for registration error on the BMH")
		Eventually(func(g Gomega) {
			key := types.NamespacedName{Namespace: bmh.Namespace, Name: bmh.Name}
			g.Expect(clusterProxy.GetClient().Get(ctx, key, &bmh)).To(Succeed())
			g.Expect(bmh.Status.ErrorType).To(Equal(metal3api.RegistrationError))
		}, e2eConfig.GetIntervals(specName, "wait-registration-error")...).Should(Succeed())
	})

	It("should inspect a newly created BMH", func() {
		By("creating a secret with BMH credentials")
		bmcCredentials := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bmc-credentials",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"username": bmcUser,
				"password": bmcPassword,
			},
		}
		err := clusterProxy.GetClient().Create(ctx, &bmcCredentials)
		Expect(err).NotTo(HaveOccurred())

		By("creating a BMH")
		bmh := metal3api.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      specName + "-inspect",
				Namespace: namespace.Name,
			},
			Spec: metal3api.BareMetalHostSpec{
				BMC: metal3api.BMCDetails{
					Address:         bmcAddress,
					CredentialsName: "bmc-credentials",
				},
				BootMode:       metal3api.Legacy,
				BootMACAddress: bootMacAddress,
			},
		}
		err = clusterProxy.GetClient().Create(ctx, &bmh)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the BMH to be in registering state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateRegistering,
		}, e2eConfig.GetIntervals(specName, "wait-registering")...)

		By("waiting for the BMH to be in inspecting state")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateInspecting,
		}, e2eConfig.GetIntervals(specName, "wait-inspecting")...)

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)
	})

	AfterEach(func() {
		cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
	})
})
