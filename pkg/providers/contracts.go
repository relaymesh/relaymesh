package providers

import "strings"

type Type string

const (
	TypeUnknown       Type = "unknown"
	TypeSCM           Type = "scm"
	TypeCollaboration Type = "collaboration"
)

type Capability string

const (
	CapabilityWebhookReceive Capability = "webhook_receive"
	CapabilityOAuthInstall   Capability = "oauth_install"
	CapabilityAPIClient      Capability = "api_client"
)

type Definition struct {
	Name         string
	Type         Type
	Capabilities []Capability
}

func (d Definition) HasCapability(capability Capability) bool {
	for _, cap := range d.Capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}

func ProviderTypeFor(name string) Type {
	if def, ok := definitionByProvider(name); ok {
		return def.Type
	}
	return TypeUnknown
}

func DefinitionFor(name string) (Definition, bool) {
	return definitionByProvider(name)
}

func definitionByProvider(name string) (Definition, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "github":
		return Definition{
			Name: "github",
			Type: TypeSCM,
			Capabilities: []Capability{
				CapabilityWebhookReceive,
				CapabilityOAuthInstall,
				CapabilityAPIClient,
			},
		}, true
	case "gitlab":
		return Definition{
			Name: "gitlab",
			Type: TypeSCM,
			Capabilities: []Capability{
				CapabilityWebhookReceive,
				CapabilityOAuthInstall,
				CapabilityAPIClient,
			},
		}, true
	case "bitbucket":
		return Definition{
			Name: "bitbucket",
			Type: TypeSCM,
			Capabilities: []Capability{
				CapabilityWebhookReceive,
				CapabilityOAuthInstall,
				CapabilityAPIClient,
			},
		}, true
	case "slack":
		return Definition{
			Name: "slack",
			Type: TypeCollaboration,
			Capabilities: []Capability{
				CapabilityWebhookReceive,
				CapabilityOAuthInstall,
			},
		}, true
	default:
		return Definition{}, false
	}
}
