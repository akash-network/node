package kube

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	executil "k8s.io/client-go/util/exec"

	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// the type implementing the interface returned by the Exec command
type execResult struct {
	exitCode int
}

func (er execResult) ExitCode() int {
	return er.exitCode
}

// a type to allow a slice of kubernetes pods to be sorted
type sortablePods []corev1.Pod

func (sp sortablePods) Len() int {
	return len(sp)
}

func (sp sortablePods) Less(i, j int) bool {
	return strings.Compare(sp[i].Name, sp[j].Name) == -1
}

func (sp sortablePods) Swap(i, j int) {
	sp[i], sp[j] = sp[j], sp[i]
}

func (c *client) Exec(ctx context.Context, leaseID mtypes.LeaseID, serviceName string, podIndex uint, cmd []string, stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer, tty bool,
	tsq remotecommand.TerminalSizeQueue) (ctypes.ExecResult, error) {
	namespace := builder.LidNS(leaseID)

	// Check that the deployment exists
	deployments, err := c.kc.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		TypeMeta:             metav1.TypeMeta{},
		LabelSelector:        "",
		FieldSelector:        "",
		Watch:                false,
		AllowWatchBookmarks:  false,
		ResourceVersion:      "",
		ResourceVersionMatch: "",
		TimeoutSeconds:       nil,
		Limit:                0,
		Continue:             "",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed getting deployments for namespace %q", err, namespace)
	}

	// If no deployments are found yet then the deployment hasn't been spun up kubernetes yet
	if 0 == len(deployments.Items) {
		return nil, cluster.ErrExecDeploymentNotYetRunning
	}

	// Check that the service named exists
	serviceExists := false
	for _, deployment := range deployments.Items {
		if serviceName == deployment.GetName() {
			serviceExists = true
		}

	}
	if !serviceExists {
		return nil, cluster.ErrExecNoServiceWithName
	}

	// Check that the pod exists
	pods, err := c.kc.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		TypeMeta:             metav1.TypeMeta{},
		LabelSelector:        fmt.Sprintf("akash.network/manifest-service=%s", serviceName),
		FieldSelector:        "",
		Watch:                false,
		AllowWatchBookmarks:  false,
		ResourceVersion:      "",
		ResourceVersionMatch: "",
		TimeoutSeconds:       nil,
		Limit:                0,
		Continue:             "",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed getting pods in namespace %q", err, namespace)
	}

	// if no pods are found yet then the deployment hasn't been spun up kubernetes yet
	if 0 == len(pods.Items) {
		return nil, cluster.ErrExecServiceNotRunning
	}

	// check that the requested pod is within the range
	if podIndex >= uint(len(pods.Items)) {
		return nil, fmt.Errorf("%w: valid range is [0, %d]", cluster.ErrExecPodIndexOutOfRange, len(pods.Items)-1)
	}

	// sort the pods, since we have no idea what order kubernetes returns them in
	podsEff := sortablePods(pods.Items)
	sort.Sort(podsEff)
	selectedPod := podsEff[podIndex]
	// validate the pod is in a state where it can be connected to
	switch selectedPod.Status.Phase {
	case corev1.PodSucceeded:
		return nil, fmt.Errorf("%w: the service has completed", cluster.ErrExecServiceNotRunning)
	case corev1.PodFailed:
		return nil, fmt.Errorf("%w: the service has failed", cluster.ErrExecServiceNotRunning)
	default:
	}
	podName := selectedPod.Name
	containerName := serviceName // Container name is always the same as the service name

	// Define the necessary runtime scheme & codec to send the request
	groupVersion := schema.GroupVersion{Group: "api", Version: "v1"}
	myScheme := runtime.NewScheme()
	err = corev1.AddToScheme(myScheme)
	if err != nil {
		return nil, err
	}
	myParameterCodec := runtime.NewParameterCodec(myScheme)
	myScheme.AddKnownTypes(groupVersion, &corev1.PodExecOptions{})

	kubeConfig := *c.kubeContentConfig // Make a local copy of the configuration
	kubeConfig.GroupVersion = &groupVersion

	codecFactory := serializer.NewCodecFactory(myScheme)
	negotiatedSerializer := runtime.NegotiatedSerializer(codecFactory)
	kubeConfig.NegotiatedSerializer = negotiatedSerializer

	kubeRestClient, err := restclient.RESTClientFor(&kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: failed getting REST client", err)
	}

	c.log.Info("Opening container shell", "namespace", namespace, "pod", podName, "container", containerName)
	if tty {
		// disable stderr if running as a TTY, results come back over stdout
		stderr = nil
	}

	const subResource = "exec" // This value copied from kubectl and never changes
	// Configure the request
	req := kubeRestClient.Post().Resource("pods").Name(podName).Namespace(namespace).SubResource(subResource)
	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdin:     stdin != nil,
		Stdout:    stdout != nil,
		Stderr:    stderr != nil,
		TTY:       tty,
	}, myParameterCodec)

	// Make the request with SPDY
	exec, err := remotecommand.NewSPDYExecutor(&kubeConfig, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("%w: execution via SPDY failed", err)
	}

	// Run, passing in the streams and everything else. This runs until the remote end closes
	// or the streams close
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             stdin,  // any reader
		Stdout:            stdout, // any writer
		Stderr:            stderr, // any writer
		Tty:               tty,
		TerminalSizeQueue: tsq,
	})
	if err == nil {
		// No error means the process returned a 0 exit code
		return execResult{exitCode: 0}, nil
	}

	// Check to see if the process ran & returned an exit code
	// If this is true, don't return an error. Something ran in the
	// container which is what this code was trying to do
	if err, ok := err.(executil.CodeExitError); ok {
		return execResult{exitCode: err.Code}, nil
	}

	// Some errors are untyped, use string matching to give better answers
	if strings.Contains(err.Error(), "error executing command in container") {
		if strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "executable file not found in $PATH") {
			return nil, cluster.ErrExecCommandDoesNotExist
		}
		// Don't send the full text of unknown errors back to the user
		// Log the error here so this can be tracked down somehow in the provider logs at least
		c.log.Error("command execution failed", "err", err)
		return nil, cluster.ErrExecCommandExecutionFailed
	}

	return nil, err
}
