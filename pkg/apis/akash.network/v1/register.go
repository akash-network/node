package v1

import (
	"context"
	"reflect"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// CRDSingular the singleton name
	CRDSingular string = "manifest"
	// CRDPlural represents CRD resource Plural
	CRDPlural string = "manifests"
	// CRDGroup represents CRD resource Group
	CRDGroup string = "akash.network"
	// CRDVersion represents CRD resource Version
	CRDVersion string = "v1"
	// FullCRDName represents CRD resource fullname
	FullCRDName string = CRDPlural + "." + CRDGroup
)

var (
	// SchemeBuilder represents new CRD scheme builder
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme applies all the stored functions to the scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// Create a  Rest client with the new CRD Schema
var SchemeGroupVersion = schema.GroupVersion{Group: CRDGroup, Version: CRDVersion}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Manifest{},
		&ManifestList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// CreateCRD creates the CRD resource, ignore error if it already exists
func CreateCRD(ctx context.Context, clientset apiextcs.Interface) error {
	crd := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: FullCRDName},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: CRDGroup,
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    CRDVersion,
					Served:  true,
					Storage: true,
					/* TODO: Re-enable once we have test around the schema format
					Schema: &apiextv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextv1.JSONSchemaProps{},
					},
					*/
					Subresources:             nil,
					AdditionalPrinterColumns: nil,
				},
			},
			Conversion: nil,
			Scope:      apiextv1.NamespaceScoped,
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural:     CRDPlural,
				Singular:   CRDSingular,
				Kind:       reflect.TypeOf(Manifest{}).Name(),
				ShortNames: []string{"ams"},
			},
		},
	}

	var err error
	_, err = clientset.ApiextensionsV1().CustomResourceDefinitions().Create(
		ctx,
		crd,
		v1.CreateOptions{},
	)
	if err != nil && k8serrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
