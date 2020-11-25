module github.com/AliyunContainerService/et-operator

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/sirupsen/logrus v1.4.2
	go.uber.org/zap v1.9.1
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975
	k8s.io/api v0.16.8
	k8s.io/apiextensions-apiserver v0.0.0
	k8s.io/apimachinery v0.16.8
	k8s.io/cli-runtime v0.16.8
	k8s.io/client-go v0.16.8
	k8s.io/code-generator v0.16.9
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.16.8
	k8s.io/utils v0.0.0-20190801114015-581e00157fb1
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/yaml v1.1.0
)

replace (
	k8s.io/api => k8s.io/api v0.16.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.8
	k8s.io/apiserver => k8s.io/apiserver v0.16.8
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.8
	k8s.io/client-go => k8s.io/client-go v0.16.8
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.8
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.8
	k8s.io/code-generator => k8s.io/code-generator v0.16.8
	k8s.io/component-base => k8s.io/component-base v0.16.8
	k8s.io/cri-api => k8s.io/cri-api v0.16.8
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.8
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.8
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.8
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.8
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.8
	k8s.io/kubectl => k8s.io/kubectl v0.16.8
	k8s.io/kubelet => k8s.io/kubelet v0.16.8
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.16.8
	k8s.io/metrics => k8s.io/metrics v0.16.8
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.16.8
)
