package builder

import (
	"fmt"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	manifesttypes "github.com/ovrclk/akash/manifest"
	clusterUtil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/sdl"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type workloadBase interface {
	builderBase
	Name() string
}

type workload struct {
	builder
	service          *manifesttypes.Service
	runtimeClassName string
}

var _ workloadBase = (*workload)(nil)

func newWorkloadBuilder(log log.Logger, settings Settings, lid mtypes.LeaseID, group *manifesttypes.Group, service *manifesttypes.Service) workload {
	return workload{
		builder: builder{
			settings: settings,
			log:      log.With("module", "kube-builder"),
			lid:      lid,
			group:    group,
		},
		service:          service,
		runtimeClassName: settings.DeploymentRuntimeClass,
	}
}

func (b *workload) container() corev1.Container {
	falseValue := false

	kcontainer := corev1.Container{
		Name:    b.service.Name,
		Image:   b.service.Image,
		Command: b.service.Command,
		Args:    b.service.Args,
		Resources: corev1.ResourceRequirements{
			Limits:   make(corev1.ResourceList),
			Requests: make(corev1.ResourceList),
		},
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &falseValue,
			Privileged:               &falseValue,
			AllowPrivilegeEscalation: &falseValue,
		},
	}

	if cpu := b.service.Resources.CPU; cpu != nil {
		requestedCPU := clusterUtil.ComputeCommittedResources(b.settings.CPUCommitLevel, cpu.Units)
		kcontainer.Resources.Requests[corev1.ResourceCPU] = resource.NewScaledQuantity(int64(requestedCPU.Value()), resource.Milli).DeepCopy()
		kcontainer.Resources.Limits[corev1.ResourceCPU] = resource.NewScaledQuantity(int64(cpu.Units.Value()), resource.Milli).DeepCopy()
	}

	if mem := b.service.Resources.Memory; mem != nil {
		requestedMem := clusterUtil.ComputeCommittedResources(b.settings.MemoryCommitLevel, mem.Quantity)
		kcontainer.Resources.Requests[corev1.ResourceMemory] = resource.NewQuantity(int64(requestedMem.Value()), resource.DecimalSI).DeepCopy()
		kcontainer.Resources.Limits[corev1.ResourceMemory] = resource.NewQuantity(int64(mem.Quantity.Value()), resource.DecimalSI).DeepCopy()
	}

	for _, ephemeral := range b.service.Resources.Storage {
		attr := ephemeral.Attributes.Find(sdl.StorageAttributePersistent)
		if persistent, _ := attr.AsBool(); !persistent {
			requestedStorage := clusterUtil.ComputeCommittedResources(b.settings.StorageCommitLevel, ephemeral.Quantity)
			kcontainer.Resources.Requests[corev1.ResourceEphemeralStorage] = resource.NewQuantity(int64(requestedStorage.Value()), resource.DecimalSI).DeepCopy()
			kcontainer.Resources.Limits[corev1.ResourceEphemeralStorage] = resource.NewQuantity(int64(ephemeral.Quantity.Value()), resource.DecimalSI).DeepCopy()

			break
		}
	}

	if b.service.Params != nil {
		for _, params := range b.service.Params.Storage {
			kcontainer.VolumeMounts = append(kcontainer.VolumeMounts, corev1.VolumeMount{
				// matches VolumeName in persistentVolumeClaims below
				Name:      fmt.Sprintf("%s-%s", b.service.Name, params.Name),
				ReadOnly:  params.ReadOnly,
				MountPath: params.Mount,
			})
		}
	}

	envVarsAdded := make(map[string]int)
	for _, env := range b.service.Env {
		parts := strings.SplitN(env, "=", 2)
		switch len(parts) {
		case 2:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0], Value: parts[1]})
		case 1:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0]})
		}
		envVarsAdded[parts[0]] = 0
	}
	kcontainer.Env = b.addEnvVarsForDeployment(envVarsAdded, kcontainer.Env)

	for _, expose := range b.service.Expose {
		kcontainer.Ports = append(kcontainer.Ports, corev1.ContainerPort{
			ContainerPort: int32(expose.Port),
		})
	}

	return kcontainer
}

func (b *workload) persistentVolumeClaims() []corev1.PersistentVolumeClaim {
	var pvcs []corev1.PersistentVolumeClaim // nolint:prealloc

	for _, storage := range b.service.Resources.Storage {
		attr := storage.Attributes.Find(sdl.StorageAttributePersistent)
		if persistent, valid := attr.AsBool(); !valid || !persistent {
			continue
		}

		volumeMode := corev1.PersistentVolumeFilesystem
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s", b.service.Name, storage.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Limits:   make(corev1.ResourceList),
					Requests: make(corev1.ResourceList),
				},
				VolumeMode:       &volumeMode,
				StorageClassName: nil,
				DataSource:       nil, // bind to existing pvc. akash does not support it. yet
			},
		}

		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.NewQuantity(int64(storage.Quantity.Value()), resource.DecimalSI).DeepCopy()

		attr = storage.Attributes.Find(sdl.StorageAttributeClass)
		if class, valid := attr.AsString(); valid && class != sdl.StorageClassDefault {
			pvc.Spec.StorageClassName = &class
		}

		pvcs = append(pvcs, pvc)
	}

	return pvcs
}

func (b *workload) Name() string {
	return b.service.Name
}

func (b *workload) labels() map[string]string {
	obj := b.builder.labels()
	obj[AkashManifestServiceLabelName] = b.service.Name
	return obj
}

func (b *workload) addEnvVarsForDeployment(envVarsAlreadyAdded map[string]int, env []corev1.EnvVar) []corev1.EnvVar {
	// Add each env. var. if it is not already set by the SDL
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashGroupSequence, b.lid.GetGSeq())
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashDeploymentSequence, b.lid.GetDSeq())
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashOrderSequence, b.lid.GetOSeq())
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashOwner, b.lid.Owner)
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashProvider, b.lid.Provider)
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashClusterPublicHostname, b.settings.ClusterPublicHostname)
	return env
}
