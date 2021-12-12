package migrate

import (
	"github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	"github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
)

func ManifestSpecFromV1(from v1.ManifestSpec) v2beta1.ManifestSpec {
	to := v2beta1.ManifestSpec{
		LeaseID: LeaseIDFromV1(from.LeaseID),
		Group:   ManifestGroupFromV1(from.Group),
	}

	return to
}

func LeaseIDFromV1(from v1.LeaseID) v2beta1.LeaseID {
	return v2beta1.LeaseID{
		Owner:    from.Owner,
		DSeq:     from.DSeq,
		GSeq:     from.GSeq,
		OSeq:     from.OSeq,
		Provider: from.Provider,
	}
}

func ManifestGroupFromV1(from v1.ManifestGroup) v2beta1.ManifestGroup {
	to := v2beta1.ManifestGroup{
		Name:     from.Name,
		Services: ManifestServiceFromV1(from.Services),
	}

	return to
}

func ManifestServiceFromV1(from []v1.ManifestService) []v2beta1.ManifestService {
	to := make([]v2beta1.ManifestService, 0, len(from))

	for _, oldSvc := range from {
		svc := v2beta1.ManifestService{
			Name:      oldSvc.Name,
			Image:     oldSvc.Image,
			Count:     oldSvc.Count,
			Args:      oldSvc.Args,
			Env:       oldSvc.Env,
			Expose:    ManifestServiceExposeFromV1(oldSvc.Expose),
			Resources: ManifestResourceUnitsFromV1(oldSvc.Resources),
			Params:    nil, // v1 does not have params section, so nil
		}

		to = append(to, svc)
	}

	return to
}

func ManifestServiceExposeFromV1(from []v1.ManifestServiceExpose) []v2beta1.ManifestServiceExpose {
	to := make([]v2beta1.ManifestServiceExpose, 0, len(from))

	for _, oldExpose := range from {
		expose := v2beta1.ManifestServiceExpose{
			Port:         oldExpose.Port,
			ExternalPort: oldExpose.ExternalPort,
			Proto:        oldExpose.Proto,
			Service:      oldExpose.Service,
			Global:       oldExpose.Global,
			Hosts:        oldExpose.Hosts,
			HTTPOptions: v2beta1.ManifestServiceExposeHTTPOptions{
				MaxBodySize: oldExpose.HTTPOptions.MaxBodySize,
				ReadTimeout: oldExpose.HTTPOptions.ReadTimeout,
				SendTimeout: oldExpose.HTTPOptions.SendTimeout,
				NextTries:   oldExpose.HTTPOptions.NextTries,
				NextTimeout: oldExpose.HTTPOptions.NextTimeout,
				NextCases:   oldExpose.HTTPOptions.NextCases,
			},
		}

		to = append(to, expose)
	}

	return to
}

func ManifestResourceUnitsFromV1(from v1.ResourceUnits) v2beta1.ResourceUnits {
	to := v2beta1.ResourceUnits{
		CPU:    from.CPU,
		Memory: from.Memory,
		Storage: []v2beta1.ManifestServiceStorage{
			{
				Name: "default",
				Size: from.Storage,
			},
		},
	}

	return to
}
