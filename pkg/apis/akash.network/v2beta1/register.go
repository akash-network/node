package v2beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	crdGroup   = "akash.network"
	crdVersion = "v2beta1"
)

var (
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme applies all the stored functions to the scheme
	AddToScheme = schemeBuilder.AddToScheme

	// SchemeGroupVersion creates a Rest client with the new CRD Schema
	SchemeGroupVersion = schema.GroupVersion{Group: crdGroup, Version: crdVersion}
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Manifest{},
		&ManifestList{},
	)
	scheme.AddKnownTypes(SchemeGroupVersion,
		&InventoryRequest{},
		&InventoryRequestList{},
		&Inventory{},
		&InventoryList{},
	)
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ProviderHost{},
		&ProviderHostList{})

	scheme.AddKnownTypes(SchemeGroupVersion,
		&ProviderLeasedIP{},
		&ProviderLeasedIPList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
