package rest

import (
	"encoding/json"
	"fmt"
	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	"github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	"github.com/ovrclk/akash/provider/cluster"
	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/tendermint/tendermint/libs/log"
	"net/http"
)

type endpointMigrateRequestBody struct {
	EndpointsToMigrate []string `json:"endpoints_to_migrate"`
	DestinationDSeq    uint64   `json:"destination_dseq"`
	DestinationGSeq    uint32   `json:"destination_gseq"`
}

type serviceExposeWithName struct {
	expose      v2beta1.ManifestServiceExpose
	serviceName string
	proto       manifest.ServiceProtocol
}

func migrateEndpointHandler(log log.Logger, clusterService cluster.Service, client cluster.Client) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		body := endpointMigrateRequestBody{}
		dec := json.NewDecoder(req.Body)
		err := dec.Decode(&body)
		defer func() {
			_ = req.Body.Close()
		}()

		if err != nil {
			rw.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		if len(body.EndpointsToMigrate) == 0 {
			log.Error("no endpoints indicated for migration")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		owner := requestOwner(req)

		found, leaseID, mgroup, err := clusterService.FindActiveLease(req.Context(),
			owner,
			body.DestinationDSeq,
			body.DestinationGSeq)

		if err != nil {
			log.Error("failed checking if destination deployment exists", "err", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !found {
			log.Info("destination deployment does not exist", "dseq", body.DestinationDSeq)
			http.Error(rw, "destination deployment does not exist", http.StatusBadRequest)
			return
		}

		neededEndpoints := make(map[string]struct{})
		for _, name := range body.EndpointsToMigrate {
			neededEndpoints[name] = struct{}{}
		}

		// verify that the destination leases can use the requested endpoints
		endpointsInDestination := make(map[string]struct{})
		var servicesToMigrate []serviceExposeWithName
		for _, service := range mgroup.Services {
			for _, expose := range service.Expose {
				if !expose.Global || len(expose.IP) == 0 {
					continue
				}
				endpointsInDestination[expose.IP] = struct{}{}

				if _, isRequested := endpointsInDestination[expose.IP]; !isRequested {
					continue
				}

				entry := serviceExposeWithName{
					expose:      expose,
					serviceName: service.Name,
				}
				// Pre-parse this before any changes are made
				entry.proto, err = manifest.ParseServiceProtocol(expose.Proto)
				if err != nil {
					log.Error("could not parse protocol from service expose", "err", err)
					rw.WriteHeader(http.StatusInternalServerError)
					return
				}
				servicesToMigrate = append(servicesToMigrate, entry)
			}
		}

		for _, endpointName := range body.EndpointsToMigrate {
			_, exists := endpointsInDestination[endpointName]
			if !exists {
				msg := fmt.Sprintf("the endpoint %q does not exist in the destination deployment", endpointName)
				http.Error(rw, msg, http.StatusBadRequest)
				return
			}
		}

		for _, serviceExpose := range servicesToMigrate {
			externalPort := serviceExpose.expose.DetermineExposedExternalPort()
			sharingKey := clusterutil.MakeIPSharingKey(leaseID, serviceExpose.expose.IP)
			err = client.DeclareIP(req.Context(),
				leaseID,
				serviceExpose.serviceName,
				uint32(serviceExpose.expose.Port),
				uint32(externalPort),
				serviceExpose.proto,
				sharingKey,
				true)

			if err != nil {
				log.Error("could not re-declare IP as part of endpoint migration", "lease", leaseID, "err", err)
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		result := make(map[string]interface{})
		result["transferred"] = body.EndpointsToMigrate
		writeJSON(log, rw, result)

	}
}
