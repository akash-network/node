package kube

import (
	"github.com/ovrclk/akash/types"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	akashDeploymentAnnotation = "akash.network/deployment"
)

type deployment struct {
	LID types.LeaseID        `json:"LeaseID"`
	MG  *types.ManifestGroup `json:"ManifestGroup"`
}

func newDeployment(lid types.LeaseID, mgroup *types.ManifestGroup) *deployment {
	return &deployment{LID: lid, MG: mgroup}
}

func (d *deployment) LeaseID() types.LeaseID {
	return d.LID
}

func (d *deployment) ManifestGroup() *types.ManifestGroup {
	return d.MG
}

func deploymentLabels() map[string]string {
	return map[string]string{
		akashManagedLabelName: "true",
	}
}

func deploymentSelector() labels.Selector {
	return labels.SelectorFromSet(deploymentLabels())
}
