package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ovrclk/akash/provider/cluster"
	clustertypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/tendermint/tendermint/libs/log"
	"net/http"
	"strings"
)

type migrateRequestBody struct {
	HostnamesToMigrate []string `json:"hostnames_to_migrate"`
	DestinationDSeq    uint64   `json:"destination_dseq"`
	DestinationGSeq    uint32   `json:"destination_gseq"`
}

func migrateHandler(log log.Logger, hostnameService clustertypes.HostnameServiceClient, clusterService cluster.Service) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		body := migrateRequestBody{}
		dec := json.NewDecoder(req.Body)
		err := dec.Decode(&body)
		defer func() {
			_ = req.Body.Close()
		}()

		if err != nil {
			log.Error("could not read request body as json", "err", err)
			rw.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		if len(body.HostnamesToMigrate) == 0 {
			log.Error("no hostnames indicated for migration")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}

		owner := requestOwner(req)

		// Make sure this hostname can be taken
		//  make sure destination deployment actually exists
		found, leaseID, mgroup, err := clusterService.FindActiveLease(req.Context(), owner, body.DestinationDSeq, body.DestinationGSeq)
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

		hostnameToServiceName := make(map[string]string)
		hostnameToExternalPort := make(map[string]uint32)

		// check that the destination leases can actually use the requested hostnames
		// for a hostname to be migrated it must be declared in the SDL
		for _, service := range mgroup.Services {
			for _, expose := range service.Expose {
				for _, host := range expose.Hosts {
					hostnameToServiceName[host] = service.Name
					port := uint32(expose.ExternalPort)
					if port == 0 {
						port = uint32(expose.Port)
					}
					hostnameToExternalPort[host] = port
				}
			}
		}

		for _, hostname := range body.HostnamesToMigrate {
			_, inUse := hostnameToServiceName[hostname]
			if !inUse {
				msg := fmt.Sprintf("the hostname %q is not used by this deployment", hostname)
				http.Error(rw, msg, http.StatusBadRequest)
				return
			}
		}

		// Tell the hostname service to move the hostnames to the new deployment, unconditionally
		log.Debug("preparing migration of hostnames", "cnt", len(body.HostnamesToMigrate))
		if err = hostnameService.PrepareHostnamesForTransfer(req.Context(), body.HostnamesToMigrate, leaseID); err != nil {
			if errors.Is(err, cluster.ErrHostnameNotAllowed) {
				log.Info("hostname not allowed", "err", err)
				http.Error(rw, err.Error(), http.StatusBadRequest)
				return
			}

			log.Error("failed preparing hostnames for transfer", "err", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Update the CRDs now
		log.Debug("transferring hostnames", "cnt", len(body.HostnamesToMigrate))

		// Migrate the hostnames
		for _, hostname := range body.HostnamesToMigrate {
			serviceName := hostnameToServiceName[hostname]
			externalPort := hostnameToExternalPort[hostname]
			err = clusterService.TransferHostname(req.Context(), leaseID, hostname, serviceName, externalPort)
			if err != nil {
				log.Error("failed starting transfer of hostnames", "err", err)
				// This errors halts the transfer and returns immediately to the client.
				// If the error is transient the client can fix this by just submitting the same request again
				msg := &strings.Builder{}
				_, _ = fmt.Fprintf(msg, "failed transferring %q: %s", hostname, err.Error())
				http.Error(rw, msg.String(), http.StatusInternalServerError)
				return
			}
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		result := make(map[string]interface{})
		result["transferred"] = body.HostnamesToMigrate
		writeJSON(log, rw, result)
	}

}
