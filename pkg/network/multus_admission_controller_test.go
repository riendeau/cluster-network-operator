package network

import (
	"testing"

	"github.com/openshift/cluster-network-operator/pkg/hypershift"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	operv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-network-operator/pkg/names"

	cnofake "github.com/openshift/cluster-network-operator/pkg/client/fake"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var MultusAdmissionControllerConfig = operv1.Network{
	Spec: operv1.NetworkSpec{
		ServiceNetwork: []string{"172.30.0.0/16"},
		ClusterNetwork: []operv1.ClusterNetworkEntry{
			{
				CIDR:       "10.128.0.0/15",
				HostPrefix: 23,
			},
		},
		DefaultNetwork: operv1.DefaultNetworkDefinition{
			Type: operv1.NetworkTypeOpenShiftSDN,
			OpenShiftSDNConfig: &operv1.OpenShiftSDNConfig{
				Mode: operv1.SDNModeNetworkPolicy,
			},
		},
	},
}

// TestRenderMultusAdmissionController has some simple rendering tests
func TestRenderMultusAdmissionController(t *testing.T) {
	g := NewGomegaWithT(t)

	crd := MultusAdmissionControllerConfig.DeepCopy()
	config := &crd.Spec
	disabled := true
	config.DisableMultiNetwork = &disabled
	fillDefaults(config, nil)

	fakeClient := cnofake.NewFakeClient(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test1-ignored",
				Labels: map[string]string{
					"openshift.io/cluster-monitoring": "true",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test2-not-ignored",
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "test3-ignored",
			Labels: map[string]string{
				"openshift.io/cluster-monitoring": "true",
			},
		},
		})
	bootstrap := fakeBootstrapResult()

	// disable MultusAdmissionController
	objs, err := renderMultusAdmissionController(config, manifestDir, false, bootstrap, fakeClient)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(objs).NotTo(ContainElement(HaveKubernetesID("Deployment", "openshift-multus", "multus-admission-controller")))

	// enable MultusAdmissionController
	enabled := false
	config.DisableMultiNetwork = &enabled
	objs, err = renderMultusAdmissionController(config, manifestDir, false, bootstrap, fakeClient)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(objs).To(ContainElement(HaveKubernetesID("Deployment", "openshift-multus", "multus-admission-controller")))

	// Check rendered object
	g.Expect(len(objs)).To(Equal(10))
	g.Expect(objs).To(ContainElement(HaveKubernetesID("Service", "openshift-multus", "multus-admission-controller")))
	g.Expect(objs).To(ContainElement(HaveKubernetesID("ClusterRole", "", "multus-admission-controller-webhook")))
	g.Expect(objs).To(ContainElement(HaveKubernetesID("ClusterRoleBinding", "", "multus-admission-controller-webhook")))
	g.Expect(objs).To(ContainElement(HaveKubernetesID("ValidatingWebhookConfiguration", "", names.MULTUS_VALIDATING_WEBHOOK)))
	g.Expect(objs).To(ContainElement(HaveKubernetesID("Deployment", "openshift-multus", "multus-admission-controller")))
}

// TestRenderMultusAdmissonControllerConfigForHyperShift has some simple rendering tests
func TestRenderMultusAdmissonControllerConfigForHyperShift(t *testing.T) {
	doTestRenderMultusAdmissonControllerConfigForHyperShift(t, false)
}

// TestRenderMultusAdmissonControllerConfigForHyperShiftWithRequests tests that changes to
// multus container resource requests are preserved
func TestRenderMultusAdmissonControllerConfigForHyperShiftWithAlteredResourceRequests(t *testing.T) {
	doTestRenderMultusAdmissonControllerConfigForHyperShift(t, true)
}

func doTestRenderMultusAdmissonControllerConfigForHyperShift(t *testing.T, addMultusDeployment bool) {
	g := NewGomegaWithT(t)

	crd := MultusAdmissionControllerConfig.DeepCopy()
	config := &crd.Spec
	disabled := true
	config.DisableMultiNetwork = &disabled
	fillDefaults(config, nil)

	clientObjs := []client.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test1-ignored",
				Labels: map[string]string{
					"openshift.io/cluster-monitoring": "true",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test2-not-ignored",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "MyCM",
				Namespace: "test1-ignored",
			},
			Data: map[string]string{
				"MyCMKey": "key",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-service-ca.crt",
				Namespace: "test1-ignored",
			},
			Data: map[string]string{
				"MyCMKey": "key",
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test3-ignored",
				Labels: map[string]string{
					"openshift.io/cluster-monitoring": "true",
				},
			},
		},
	}

	hsc := hypershift.NewHyperShiftConfig()
	hsc.Enabled = true
	hsc.CAConfigMap = "MyCM"
	hsc.CAConfigMapKey = "MyCMKey"
	hsc.Name = "MyCluster"
	hsc.Namespace = "test1-ignored"
	hsc.RunAsUser = "1001"
	hsc.ReleaseImage = "MyImage"
	hsc.ControlPlaneImage = "MyCPOImage"

	expectedCpuReq := "10m"
	expectedMemReq := "50Mi"
	if addMultusDeployment {
		expectedCpuReq = "20m"
		expectedMemReq = "75Mi"
		clientObjs = append(clientObjs, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multus-admission-controller",
				Namespace: hsc.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "multus-admission-controller",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(expectedCpuReq),
									corev1.ResourceMemory: resource.MustParse(expectedMemReq),
								},
							},
						}},
					},
				},
			},
		})
	}

	fakeClient := cnofake.NewFakeClient(clientObjs...)
	bootstrap := fakeBootstrapResultWithHyperShift()

	objs, err := renderMultusAdmissonControllerConfig(manifestDir, false, bootstrap, fakeClient, hsc, "")
	g.Expect(err).NotTo(HaveOccurred())

	// Check rendered object
	for _, obj := range objs {
		switch {
		case obj.GetKind() == "Service" && obj.GetName() == "multus-admission-controller":
			labels := obj.GetLabels()
			g.Expect(len(labels)).To(Equal(2))
			g.Expect(labels["hypershift.openshift.io/allow-guest-webhooks"]).To(Equal("true"))

			annotations := obj.GetAnnotations()
			g.Expect(len(annotations)).To(Equal(1))
			g.Expect(annotations["network.operator.openshift.io/cluster-name"]).To(Equal("management"))

		case obj.GetKind() == "Deployment" && obj.GetName() == "multus-admission-controller":
			// Check the resource request values
			containers, found, err := uns.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(found).To(BeTrue())

			var mac map[string]interface{}
			for _, container := range containers {
				cmap := container.(map[string]interface{})
				name, found, err := uns.NestedString(cmap, "name")
				if found && err == nil && name == "multus-admission-controller" {
					mac = cmap
					break
				}
			}
			g.Expect(mac).ToNot(BeNil())

			cpuReq, found, err := uns.NestedString(mac, "resources", "requests", "cpu")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(found).To(BeTrue())
			g.Expect(cpuReq).To(Equal(expectedCpuReq))

			memReq, found, err := uns.NestedString(mac, "resources", "requests", "memory")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(found).To(BeTrue())
			g.Expect(memReq).To(Equal(expectedMemReq))
		}
	}
}

// TestRenderMultusAdmissionControllerGetNamespace tests getOpenshiftNamespaces()
func TestRenderMultusAdmissionControllerGetNamespace(t *testing.T) {
	g := NewGomegaWithT(t)

	fakeClient := cnofake.NewFakeClient(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test1-ignored",
				Labels: map[string]string{
					"openshift.io/cluster-monitoring": "true",
				},
				Annotations: map[string]string{
					"workload.openshift.io/allowed": "management",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test2-not-ignored",
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "test3-ignored",
			Labels: map[string]string{
				"openshift.io/cluster-monitoring": "true",
			},
			Annotations: map[string]string{
				"workload.openshift.io/allowed": "management",
			},
		},
		})
	namespaces, err := getOpenshiftNamespaces(fakeClient)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(namespaces).To(Equal("test1-ignored,test3-ignored"))
}
