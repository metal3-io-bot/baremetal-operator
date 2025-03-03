package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"

	capm3_e2e "github.com/metal3-io/cluster-api-provider-metal3/test/e2e"

	metal3api "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

var _ = Describe("basic", func() {
	var (
		specName       = "basic-ops"
		namespace      *corev1.Namespace
		cancelWatches  context.CancelFunc
		bmcUser        string
		bmcPassword    string
		bmcAddress     string
		bootMacAddress string
	)
	const (
		rebootAnnotation   = "reboot.metal3.io"
		poweroffAnnotation = "reboot.metal3.io/poweroff"
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

	It("should control power cycle of BMH though annotations", func() {
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
				Name:      specName + "-powercycle",
				Namespace: namespace.Name,
				Annotations: map[string]string{
					"inspect.metal3.io": "disabled",
				},
			},
			Spec: metal3api.BareMetalHostSpec{
				Online: true,
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

		By("waiting for the BMH to become available")
		WaitForBmhInProvisioningState(ctx, WaitForBmhInProvisioningStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  metal3api.StateAvailable,
		}, e2eConfig.GetIntervals(specName, "wait-available")...)

		By("setting the reboot annotation and checking that the BMH was rebooted")
		capm3_e2e.AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, rebootAnnotation, pointer.String("{\"force\": true}"))

		WaitForBmhInPowerState(ctx, WaitForBmhInPowerStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  PoweredOff,
		}, e2eConfig.GetIntervals(specName, "wait-power-state")...)

		WaitForBmhInPowerState(ctx, WaitForBmhInPowerStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  PoweredOn,
		}, e2eConfig.GetIntervals(specName, "wait-power-state")...)

		By("setting the power off annotation on the BMH and checking that it worked")
		capm3_e2e.AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, poweroffAnnotation, pointer.String("{\"force\": true}"))

		WaitForBmhInPowerState(ctx, WaitForBmhInPowerStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  PoweredOff,
		}, e2eConfig.GetIntervals(specName, "wait-power-state")...)

		// power on
		By("removing the power off annotation and checking that the BMH powers on")
		capm3_e2e.AnnotateBmh(ctx, clusterProxy.GetClient(), bmh, poweroffAnnotation, nil)

		WaitForBmhInPowerState(ctx, WaitForBmhInPowerStateInput{
			Client: clusterProxy.GetClient(),
			Bmh:    bmh,
			State:  PoweredOn,
		}, e2eConfig.GetIntervals(specName, "wait-power-state")...)
	})

	AfterEach(func() {
		cleanup(ctx, clusterProxy, namespace, cancelWatches, e2eConfig.GetIntervals("default", "wait-namespace-deleted")...)
	})
})
