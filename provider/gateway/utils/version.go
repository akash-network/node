package utils

import (
	"github.com/cosmos/cosmos-sdk/version"
)

type AkashVersionInfo struct {
	Version          string `json:"version"`
	GitCommit        string `json:"commit"`
	BuildTags        string `json:"buildTags"`
	GoVersion        string `json:"go"`
	CosmosSdkVersion string `json:"cosmosSdkVersion"`
}

func NewAkashVersionInfo() AkashVersionInfo {
	verInfo := version.NewInfo()
	return AkashVersionInfo{
		Version:          verInfo.Version,
		GitCommit:        verInfo.GitCommit,
		BuildTags:        verInfo.BuildTags,
		GoVersion:        verInfo.GoVersion,
		CosmosSdkVersion: verInfo.CosmosSdkVersion,
	}
}
