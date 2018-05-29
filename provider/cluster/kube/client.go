package kube

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client interface {
	cluster.Client
}

type client struct {
	kc kubernetes.Interface
}

func NewClient() (Client, error) {

	kubeconfig := path.Join(homedir.HomeDir(), ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("error building config flags: %v", err)
	}

	kc, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating kubernetes client: %v", err)
	}

	_, err = kc.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error connecting to kubernetes: %v", err)
	}

	return &client{
		kc: kc,
	}, nil

}

func (c *client) Deploy(oid types.OrderID, group *types.ManifestGroup) error {
	if err := c.deployNS(oid); err != nil {
		return err
	}
	if err := c.deployServices(oid, group); err != nil {
		return err
	}
	return nil
}

func (c *client) deployNS(oid types.OrderID) error {
	_, err := c.kc.CoreV1().Namespaces().Create(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: oidNS(oid),
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (c *client) deployServices(oid types.OrderID, group *types.ManifestGroup) error {

	// single service, single expose.
	if len(group.Services) == 0 {
		return fmt.Errorf("no services")
	}
	gsvc := group.Services[0]

	if len(gsvc.Expose) == 0 {
		return fmt.Errorf("no expose")
	}
	gexpose := gsvc.Expose[0]

	kns := oidNS(oid)

	// create kube deployment
	kcontainer := corev1.Container{
		Name:  gsvc.Name,
		Image: gsvc.Image,
		Args:  gsvc.Args,
	}

	for _, env := range gsvc.Env {
		parts := strings.Split(env, "=")
		switch len(parts) {
		case 2:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0], Value: parts[1]})
		case 1:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0]})
		}
	}

	kcontainer.Ports = append(kcontainer.Ports, corev1.ContainerPort{
		ContainerPort: int32(gexpose.Port),
	})

	labels := map[string]string{
		"akash.io/service-name": gsvc.Name,
	}

	kdeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kns,
			Name:      gsvc.Name,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{kcontainer},
				},
			},
		},
	}

	_, err := c.kc.AppsV1().Deployments(kns).Create(kdeployment)
	if err != nil {
		return err
	}

	// create kube service

	ksvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kns,
			Name:      gsvc.Name,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
		},
	}

	kport := corev1.ServicePort{
		Name:       strconv.Itoa(int(gexpose.Port)),
		TargetPort: intstr.FromInt(int(gexpose.Port)),
	}

	if gexpose.ExternalPort == 0 {
		kport.Port = int32(gexpose.Port)
	} else {
		kport.Port = int32(gexpose.ExternalPort)
	}
	ksvc.Spec.Ports = append(ksvc.Spec.Ports, kport)

	_, err = c.kc.CoreV1().Services(kns).Create(ksvc)
	if err != nil {
		return err
	}

	// create ingress

	king := &extv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kns,
			Name:      gsvc.Name,
			Labels:    labels,
		},
		Spec: extv1.IngressSpec{
			Backend: &extv1.IngressBackend{
				ServiceName: gsvc.Name,
				ServicePort: intstr.FromInt(int(ksvc.Spec.Ports[0].Port)),
			},
		},
	}

	for _, host := range gexpose.Hosts {
		king.Spec.Rules = append(king.Spec.Rules, extv1.IngressRule{
			Host: host,
		})
	}

	_, err = c.kc.ExtensionsV1beta1().Ingresses(kns).Create(king)
	if err != nil {
		return err
	}

	return err
}

func oidNS(oid types.OrderID) string {
	path := strings.Replace(keys.OrderID(oid).Path(), "/", ".", -1)
	sha := sha1.Sum([]byte(path))
	return hex.EncodeToString(sha[:])
}
