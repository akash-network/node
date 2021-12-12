package builder

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	manitypes "github.com/ovrclk/akash/manifest/v2beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type PspRestricted interface {
	builderBase
	Name() string
	Create() (*v1beta1.PodSecurityPolicy, error)
	Update(obj *v1beta1.PodSecurityPolicy) (*v1beta1.PodSecurityPolicy, error)
}

type pspRestricted struct {
	builder
}

func BuildPSP(settings Settings, lid mtypes.LeaseID, group *manitypes.Group) PspRestricted { // nolint:golint,unparam
	return &pspRestricted{builder: builder{settings: settings, lid: lid, group: group}}
}

func (p *pspRestricted) Name() string {
	return p.NS()
}

func (p *pspRestricted) Create() (*v1beta1.PodSecurityPolicy, error) { // nolint:golint,unparam
	falseVal := false
	return &v1beta1.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name(),
			Namespace: p.Name(),
			Labels:    p.labels(),
			Annotations: map[string]string{
				"seccomp.security.alpha.kubernetes.io/allowedProfileNames": "docker/default,runtime/default",
				"apparmor.security.beta.kubernetes.io/allowedProfileNames": "runtime/default",
				"seccomp.security.alpha.kubernetes.io/defaultProfileName":  "runtime/default",
				"apparmor.security.beta.kubernetes.io/defaultProfileName":  "runtime/default",
			},
		},
		Spec: v1beta1.PodSecurityPolicySpec{
			Privileged:               false,
			AllowPrivilegeEscalation: &falseVal,
			RequiredDropCapabilities: []corev1.Capability{
				"ALL",
			},
			Volumes: []v1beta1.FSType{
				v1beta1.EmptyDir,
				v1beta1.PersistentVolumeClaim, // evaluate necessity later
			},
			HostNetwork: false,
			HostIPC:     false,
			HostPID:     false,
			RunAsUser: v1beta1.RunAsUserStrategyOptions{
				// fixme(#946): previous value RunAsUserStrategyMustRunAsNonRoot was interfering with
				// (b *deployment) create() RunAsNonRoot: false
				// allow any user at this moment till revise all security debris of kube api
				Rule: v1beta1.RunAsUserStrategyRunAsAny,
			},
			SELinux: v1beta1.SELinuxStrategyOptions{
				Rule: v1beta1.SELinuxStrategyRunAsAny,
			},
			SupplementalGroups: v1beta1.SupplementalGroupsStrategyOptions{
				Rule: v1beta1.SupplementalGroupsStrategyRunAsAny,
			},
			FSGroup: v1beta1.FSGroupStrategyOptions{
				Rule: v1beta1.FSGroupStrategyMustRunAs,
				Ranges: []v1beta1.IDRange{
					{
						Min: int64(1),
						Max: int64(65535),
					},
				},
			},
			ReadOnlyRootFilesystem: false,
		},
	}, nil
}

func (p *pspRestricted) Update(obj *v1beta1.PodSecurityPolicy) (*v1beta1.PodSecurityPolicy, error) { // nolint:golint,unparam
	obj.Name = p.Name()
	obj.Labels = p.labels()
	return obj, nil
}
