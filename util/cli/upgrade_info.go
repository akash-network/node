package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/google/go-github/v62/github"
	"github.com/gregjones/httpcache"
	"golang.org/x/oauth2"
)

// UpgradeInfo is expected format for the info field to allow auto-download
type UpgradeInfo struct {
	Binaries map[string]string `json:"binaries"`
	Configs  map[string]string `json:"configs,omitempty"`
	// ReserveSource names the account that funds the x/sink launch reserve. Only
	// the upgrade that installs x/sink reads it, and it halts without it.
	ReserveSource string `json:"reserve_source,omitempty"`
}

// UpgradeInfoFromTag generate upgrade info from give tag
// tag - release tag
// reserveSource - x/sink launch reserve source, or empty for other upgrades
// pretty - either prettify (true) json output or not (false)
func UpgradeInfoFromTag(ctx context.Context, tag string, reserveSource string, pretty bool) (string, error) {
	if reserveSource != "" {
		if _, err := sdk.AccAddressFromBech32(reserveSource); err != nil {
			return "", fmt.Errorf("invalid reserve source: %w", err)
		}
	}

	tc := &http.Client{
		Transport: &oauth2.Transport{
			Base: httpcache.NewMemoryCacheTransport(),
			Source: oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
			),
		},
	}

	gh := github.NewClient(tc)

	rel, resp, err := gh.Repositories.GetReleaseByTag(ctx, "akash-network", "node", tag)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("no release for tag %s", tag)
	}

	sTag := strings.TrimPrefix(tag, "v")
	checksumsAsset := fmt.Sprintf("akash_%s_checksums.txt", sTag)
	var checksumsID int64
	for _, asset := range rel.Assets {
		if asset.GetName() == checksumsAsset {
			checksumsID = asset.GetID()
		}
	}

	body, _, err := gh.Repositories.DownloadReleaseAsset(ctx, "akash-network", "node", checksumsID, http.DefaultClient)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = body.Close()
	}()

	info := &UpgradeInfo{
		Binaries:      make(map[string]string),
		ReserveSource: reserveSource,
	}

	urlBase := fmt.Sprintf("https://github.com/akash-network/node/releases/download/%s", tag)
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		tuple := strings.Split(scanner.Text(), "  ")
		if len(tuple) != 2 {
			return "", fmt.Errorf("invalid checksum format")
		}

		link := fmt.Sprintf("%s/%s?checksum=sha256:%s", urlBase, tuple[1], tuple[0])

		switch tuple[1] {
		case "akash_linux_amd64.zip":
			info.Binaries["linux/amd64"] = link
		case "akash_linux_arm64.zip":
			info.Binaries["linux/arm64"] = link
		case "akash_darwin_all.zip":
			info.Binaries["darwin/amd64"] = link
			info.Binaries["darwin/arm64"] = link
		}
	}

	var res []byte

	if pretty {
		res, err = json.MarshalIndent(info, "", "    ")
	} else {
		res, err = json.Marshal(info)
	}

	if err != nil {
		return "", err
	}

	return string(res), nil
}
