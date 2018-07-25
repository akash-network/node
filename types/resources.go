package types

var (
	_ ResourceList = &DeploymentGroup{}
	_ ResourceList = &ManifestGroup{}
)

type ResourceList interface {
	GetName() string
	GetResources() []ResourceGroup
}

func (m *ManifestGroup) GetResources() []ResourceGroup {
	resources := make([]ResourceGroup, 0, len(m.Services))
	for _, svc := range m.Services {
		resources = append(resources, ResourceGroup{
			Unit:  *svc.Unit,
			Count: svc.Count,
		})
	}
	return resources
}
