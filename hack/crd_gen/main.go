package main

import (
	"k8s.io/cli-runtime/pkg/resource"

	"flag"
	"fmt"
	"io"
	apiextinternal "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"os"
	"sigs.k8s.io/yaml"
)

func main() {
	var input string
	var output string
	flag.StringVar(&input, "file", "./config/crd/bases/kai.alibabacloud.com_trainingjobs.yaml", "input crd file")
	flag.StringVar(&output, "outfile", "", "help message for flagname")

	if output == "" {
		output = input
	}

	s := scheme.Scheme
	builder := resource.NewBuilder(noopClientGetter{}).Local()

	v1.AddToScheme(s)
	v1beta1.AddToScheme(s)
	apiextinternal.AddToScheme(s)
	result := builder.
		WithScheme(s, s.PrioritizedVersionsAllGroups()...).
		Path(false, input).
		Flatten().
		ContinueOnError().Do()

	items, err := result.Infos()
	if err != nil {
		fmt.Println("infos error:", err)
		return
	}

	crds := []*v1.CustomResourceDefinition{}
	for _, item := range items {
		crd := item.Object.(*v1.CustomResourceDefinition)
		for i, _ := range crd.Spec.Versions {
			crdVersion := crd.Spec.Versions[i]
			setProperty(crdVersion.Schema.OpenAPIV3Schema, setXPreserveUnknownFields, "spec", "etReplicaSpecs", "launcher", "template", "metadata")
			setProperty(crdVersion.Schema.OpenAPIV3Schema, setXPreserveUnknownFields, "spec", "etReplicaSpecs", "worker", "template", "metadata")
			crd.Spec.Versions[i] = crdVersion
		}
		crd.Spec.Conversion = nil
		crd.Status.StoredVersions = []string{}
		crds = append(crds, crd)
	}

	out, err := os.Create(output)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	for _, obj := range crds {
		yamlContent, err := yaml.Marshal(obj)
		if err != nil {
			panic(err)
		}
		n, err := out.Write(append([]byte("\n---\n"), yamlContent...))
		if err != nil {
			panic(err)
		}
		if n < len(yamlContent) {
			panic(io.ErrShortWrite)
		}
	}

}

func setXPreserveUnknownFields(props v1.JSONSchemaProps) v1.JSONSchemaProps {
	props.XPreserveUnknownFields = boolPtr(true)
	return props
}

func setProperty(props *v1.JSONSchemaProps, setter func(v1.JSONSchemaProps) v1.JSONSchemaProps, keys ...string) {
	if props == nil {
		return
	}
	p := props
	lastKeyInbdex := len(keys) - 1
	for i, key := range keys {
		if i == lastKeyInbdex {
			p.Properties[key] = setter(p.Properties[key])
			break
		}
		if v, ok := p.Properties[key]; ok {
			p = &v
		} else {
			break
		}
	}
	return
}

func boolPtr(v bool) *bool {
	return &v
}

// noopClientGetter implements RESTClientGetter returning only errors.
// used as a dummy getter in a local-only builder.
type noopClientGetter struct{}

func (noopClientGetter) ToRESTConfig() (*rest.Config, error) {
	return nil, fmt.Errorf("local operation only")
}
func (noopClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, fmt.Errorf("local operation only")
}
func (noopClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return nil, fmt.Errorf("local operation only")
}
