package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func logsCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "logs <service> <lease>",
		Short: "Service logs",
		Args:  cobra.ExactArgs(2),
		RunE:  session.WithSession(session.RequireNode(logs)),
	}

	session.AddFlagNode(cmd, cmd.PersistentFlags())
	cmd.Flags().Int64P("lines", "l", 10, "Number of lines from the end of the logs to show per service")
	cmd.Flags().BoolP("follow", "f", false, "Follow the log stream of the service")

	return cmd
}

func logs(session session.Session, cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	leasePath := args[1]
	lease, err := keys.ParseLeasePath(leasePath)
	if err != nil {
		return err
	}
	tailLines, err := cmd.Flags().GetInt64("lines")
	if err != nil {
		return err
	}
	follow, err := cmd.Flags().GetBool("follow")
	if err != nil {
		return err
	}

	provider, err := session.QueryClient().Provider(session.Ctx(), lease.Provider)
	if err != nil {
		return err
	}

	options := types.LogOptions{
		TailLines: tailLines,
		Follow:    follow,
	}
	b, err := json.Marshal(options)
	if err != nil {
		return err
	}

	url := provider.HostURI + "/logs/" + leasePath + "/" + serviceName
	body, err := stream(session.Ctx(), url, b)
	if err != nil {
		return err
	}
	defer body.Close()

	return printLog(session, body)
}

func printLog(session session.Session, r io.Reader) error {

	var (
		err error
		obj types.LogResponse
	)

	for dec := json.NewDecoder(r); ; {

		if err = dec.Decode(&obj); err != nil {
			break
		}

		fmt.Printf("[%v] %v\n", obj.Result.Name, obj.Result.Message)
		obj.Reset()
	}

	if err != io.EOF && err != context.Canceled {
		session.Log().Error(err.Error())
		return err
	}

	return nil

}

func stream(ctx context.Context, url string, data []byte) (io.ReadCloser, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Custom-Header", "Akash")
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("response not ok: " + resp.Status)
	}
	return resp.Body, nil
}
