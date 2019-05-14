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
	"github.com/ovrclk/dsky"
	"github.com/spf13/cobra"
)

func logsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs <service> <lease>",
		Short: "Service logs",
		RunE:  session.WithSession(session.RequireNode(logs)),
	}

	session.AddFlagNode(cmd, cmd.PersistentFlags())
	cmd.Flags().Int64P("lines", "l", 10, "Number of lines from the end of the logs to show per service")
	cmd.Flags().BoolP("follow", "f", false, "Follow the log stream of the service")
	return cmd
}

func logs(session session.Session, cmd *cobra.Command, args []string) error {
	var serviceName, leasePath string
	if len(args) > 0 {
		serviceName = args[0]
	}
	serviceName = session.Mode().Ask().StringVar(serviceName, "Service Name (required): ", true)
	if len(args) > 1 {
		leasePath = args[1]
	}
	leasePath = session.Mode().Ask().StringVar(leasePath, "Lease Path (ID) (required): ", true)

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
		switch session.Mode().Type() {
		case dsky.ModeTypeInteractive:
			msg := fmt.Sprintf("[%v] %v\n", obj.Result.Name, obj.Result.Message)
			fmt.Printf(msg)
			obj.Reset()
		default:
			dat := session.Mode().Printer().NewSection("Log").NewData()
			m := make(map[string]string)
			m[obj.Result.Name] = obj.Result.Message
			dat.Add("item", m)
			session.Mode().Printer().Flush()
		}
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
