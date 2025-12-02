package query

import (
	"strings"
	"testing"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/testutil"
)

func TestDeploymentFiltersAccept(t *testing.T) {
	owner := testutil.AccAddress(t)
	otherOwner := testutil.AccAddress(t)

	deployment := v1.Deployment{
		ID: v1.DeploymentID{
			Owner: owner.String(),
			DSeq:  100,
		},
		State: v1.DeploymentActive,
	}

	tests := []struct {
		name         string
		filters      DeploymentFilters
		deployment   v1.Deployment
		isValidState bool
		expected     bool
	}{
		{
			name:         "empty owner and invalid state accepts all",
			filters:      DeploymentFilters{},
			deployment:   deployment,
			isValidState: false,
			expected:     true,
		},
		{
			name: "empty owner with matching state",
			filters: DeploymentFilters{
				State: v1.DeploymentActive,
			},
			deployment:   deployment,
			isValidState: true,
			expected:     true,
		},
		{
			name: "empty owner with non-matching state",
			filters: DeploymentFilters{
				State: v1.DeploymentClosed,
			},
			deployment:   deployment,
			isValidState: true,
			expected:     false,
		},
		{
			name: "matching owner with invalid state",
			filters: DeploymentFilters{
				Owner: owner,
			},
			deployment:   deployment,
			isValidState: false,
			expected:     true,
		},
		{
			name: "non-matching owner with invalid state",
			filters: DeploymentFilters{
				Owner: otherOwner,
			},
			deployment:   deployment,
			isValidState: false,
			expected:     false,
		},
		{
			name: "matching owner and matching state",
			filters: DeploymentFilters{
				Owner: owner,
				State: v1.DeploymentActive,
			},
			deployment:   deployment,
			isValidState: true,
			expected:     true,
		},
		{
			name: "matching owner but non-matching state",
			filters: DeploymentFilters{
				Owner: owner,
				State: v1.DeploymentClosed,
			},
			deployment:   deployment,
			isValidState: true,
			expected:     false,
		},
		{
			name: "non-matching owner with matching state",
			filters: DeploymentFilters{
				Owner: otherOwner,
				State: v1.DeploymentActive,
			},
			deployment:   deployment,
			isValidState: true,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filters.Accept(tt.deployment, tt.isValidState)
			if got != tt.expected {
				t.Errorf("DeploymentFilters.Accept() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDeploymentString(t *testing.T) {
	owner := testutil.AccAddress(t)

	deployment := Deployment{
		Deployment: v1.Deployment{
			ID: v1.DeploymentID{
				Owner: owner.String(),
				DSeq:  100,
			},
			State: v1.DeploymentActive,
			Hash:  []byte("testhash"),
		},
		Groups: nil,
	}

	got := deployment.String()

	if !strings.Contains(got, owner.String()) {
		t.Errorf("Deployment.String() should contain owner, got %v", got)
	}
	if !strings.Contains(got, "100") {
		t.Errorf("Deployment.String() should contain DSeq, got %v", got)
	}
}

func TestDeploymentsString(t *testing.T) {
	owner := testutil.AccAddress(t)

	deployments := Deployments{
		{
			Deployment: v1.Deployment{
				ID: v1.DeploymentID{
					Owner: owner.String(),
					DSeq:  100,
				},
				State: v1.DeploymentActive,
			},
		},
		{
			Deployment: v1.Deployment{
				ID: v1.DeploymentID{
					Owner: owner.String(),
					DSeq:  200,
				},
				State: v1.DeploymentClosed,
			},
		},
	}

	got := deployments.String()

	if !strings.Contains(got, "100") {
		t.Errorf("Deployments.String() should contain first DSeq, got %v", got)
	}
	if !strings.Contains(got, "200") {
		t.Errorf("Deployments.String() should contain second DSeq, got %v", got)
	}
}

func TestDeploymentsStringEmpty(t *testing.T) {
	deployments := Deployments{}
	got := deployments.String()

	if got != "" {
		t.Errorf("Deployments.String() for empty slice = %v, want empty string", got)
	}
}

func TestDeploymentsStringSingle(t *testing.T) {
	owner := testutil.AccAddress(t)

	deployments := Deployments{
		{
			Deployment: v1.Deployment{
				ID: v1.DeploymentID{
					Owner: owner.String(),
					DSeq:  100,
				},
			},
		},
	}

	got := deployments.String()

	if !strings.Contains(got, "100") {
		t.Errorf("Deployments.String() should contain DSeq, got %v", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("Deployments.String() should not end with separator for single item")
	}
}

