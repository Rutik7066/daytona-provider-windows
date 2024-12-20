package types

import (
	"encoding/json"

	"github.com/daytonaio/daytona/pkg/provider"
)

type TargetOptions struct {
	RemoteHostname   *string `json:"Remote Hostname,omitempty"`
	RemotePort       *int    `json:"Remote Port,omitempty"`
	RemoteUser       *string `json:"Remote User,omitempty"`
	RemotePassword   *string `json:"Remote Password,omitempty"`
	RemotePrivateKey *string `json:"Remote Private Key Path,omitempty"`
	SockPath         *string `json:"Sock Path,omitempty"`
	WorkspaceDataDir *string `json:"Workspace Data Dir,omitempty"`
}

func GetTargetManifest() *provider.ProviderTargetManifest {
	return &provider.ProviderTargetManifest{
		"Remote Hostname": provider.ProviderTargetProperty{
			Type:              provider.ProviderTargetPropertyTypeString,
			DefaultValue:      "localhost",
			DisabledPredicate: "^local$",
		},
		"Remote Port": provider.ProviderTargetProperty{
			Type:              provider.ProviderTargetPropertyTypeInt,
			DefaultValue:      "2223",
			DisabledPredicate: "^local$",
		},
		"Remote User": provider.ProviderTargetProperty{
			Type:              provider.ProviderTargetPropertyTypeString,
			DefaultValue:      "Docker",
			Description:       "Note: non-root user required",
			DisabledPredicate: "^local$",
		},
		"Remote Password": provider.ProviderTargetProperty{
			Type:              provider.ProviderTargetPropertyTypeString,
			DefaultValue:      "daytona123",
			DisabledPredicate: "^local$",
			InputMasked:       true,
		},
		"Sock Path": provider.ProviderTargetProperty{
			Type:         provider.ProviderTargetPropertyTypeString,
			DefaultValue: "/var/run/docker.sock",
		},
	}
}

func ParseTargetOptions(optionsJson string) (*TargetOptions, error) {
	var targetOptions TargetOptions
	err := json.Unmarshal([]byte(optionsJson), &targetOptions)
	if err != nil {
		return nil, err
	}

	return &targetOptions, nil
}
