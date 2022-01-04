// +k8s:deepcopy-gen=package
// +k8s:defaulter-gen=TypeMeta
// +k8s:openapi-gen=true
// +groupName=akash.network

// Package v2beta1 is the initial version of types which integrate with the Kubernetes API.
//
// Contains the Stack Definition Language(pkg: github.com/ovrclk/akash/sdl) Manifest
// declarations which are written to Kubernetes CRDs for storage.
//
// Manifest {
//	ManifestSpec {
//		k8s.TypeMeta
//		LeaseID
//		ManifestGroup
//			k8s.TypeMeta
//			Name
//			[]*ManifestService
//				ManifestService analogous to a running container.
//	ManifestStatus
//		State
//		Message
// }
//
package v2beta1
