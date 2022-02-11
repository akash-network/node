package kube

import (
	"bytes"
	"context"
	"errors"
	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	akashclient_fake "github.com/ovrclk/akash/pkg/client/clientset/versioned/fake"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"testing"
	"time"

	kubefake "k8s.io/client-go/kubernetes/fake"
)

const (
	execTestServiceName = "web"
)

var errNoSPDYInTest = errors.New("SPDY connections blocked in test")

func TestSortablePodsSorting(t *testing.T) {
	v := sortablePods{
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "z"}},
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
	}

	sort.Sort(v)

	require.Equal(t, "a", v[0].Name)
	require.Equal(t, "b", v[1].Name)
	require.Equal(t, "z", v[2].Name)
}

func TestSortablePodsAlreadySorted(t *testing.T) {
	v := sortablePods{
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
	}

	sort.Sort(v)

	require.Equal(t, "a", v[0].Name)
	require.Equal(t, "b", v[1].Name)
	require.Equal(t, "c", v[2].Name)
}

func TestExecResultImpl(t *testing.T) {
	v := execResult{exitCode: 133}

	require.Equal(t, 133, v.ExitCode())
}

type execScaffold struct {
	settings builder.Settings
	leaseID  mtypes.LeaseID

	deploymentSDL sdl.SDL
	crdManifest   *crd.Manifest

	akashFake *akashclient_fake.Clientset
	kubeFake  *kubefake.Clientset

	client Client

	ctx context.Context
}

func withExecTestScaffold(t *testing.T, changePod func(pod *corev1.Pod) error, test func(s *execScaffold)) {
	s := &execScaffold{}

	s.leaseID = testutil.LeaseID(t)
	s.settings = builder.Settings{
		DeploymentIngressStaticHosts: true,
		DeploymentIngressDomain:      "*.foo.bar.com",
	}

	settingCtx := context.WithValue(context.Background(), builder.SettingsKey, s.settings)
	var cancel context.CancelFunc
	s.ctx, cancel = context.WithTimeout(settingCtx, 30*time.Second)
	defer cancel()

	deploymentPath, err := filepath.Abs("../../../x/deployment/testdata/deployment-v2.yaml")
	require.NoError(t, err)

	s.deploymentSDL, err = sdl.ReadFile(deploymentPath)
	require.NoError(t, err)
	require.NotNil(t, s.deploymentSDL)

	mani, err := s.deploymentSDL.Manifest()
	require.NoError(t, err)
	require.Len(t, mani, 1)

	s.crdManifest, err = crd.NewManifest(testKubeClientNs, s.leaseID, &mani[0])
	require.NoError(t, err)
	require.NotNil(t, s.crdManifest)

	s.akashFake = akashclient_fake.NewSimpleClientset(s.crdManifest)
	s.kubeFake = kubefake.NewSimpleClientset()

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "testpod0",
			Labels: map[string]string{
				"akash.network/manifest-service": execTestServiceName,
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	if changePod != nil {
		err = changePod(pod)
		require.NoError(t, err)
	}

	_, err = s.kubeFake.CoreV1().Pods(s.crdManifest.Name).Create(s.ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err)

	myLog := testutil.Logger(t)

	s.client = &client{
		kc:  s.kubeFake,
		ac:  s.akashFake,
		ns:  testKubeClientNs,
		log: myLog.With("mode", "test-kube-provider-client"),
		kubeContentConfig: &rest.Config{
			/**
			The Transport and Dial members of this aren't used because a SPDY transport is created. The only
			opportunity to hijack that is in the Proxy function
			*/
			Host:      "localhost:1234", // Never connected to, just needs to be something valid
			APIPath:   "/client-exec-test",
			UserAgent: "client_exec_test.go",
			Username:  "theusername",
			Password:  "thepassword",
			Proxy: func(req *http.Request) (*url.URL, error) {
				return nil, errNoSPDYInTest
			},
		},
	}

	test(s)
}

func TestClientExec(t *testing.T) {
	withExecTestScaffold(t, nil, func(s *execScaffold) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		result, err := s.client.Exec(s.ctx,
			s.leaseID,
			"web",
			0,
			[]string{"/bin/true"},
			nil,
			stdout,
			stderr,
			false,
			nil)
		// The arguments are completed valid, so we expect the code to try & establish a SPDY connection
		// which has been hijacked & blocked by the scaffold
		require.Error(t, err)
		require.Contains(t, err.Error(), "SPDY connections blocked")
		require.Nil(t, result)
	})
}

func TestClientExecTty(t *testing.T) {
	withExecTestScaffold(t, nil, func(s *execScaffold) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		stdin := &bytes.Buffer{}

		result, err := s.client.Exec(s.ctx,
			s.leaseID,
			"web",
			0,
			[]string{"/bin/true"},
			stdin,
			stdout,
			stderr,
			true,
			nil)
		// The arguments are completed valid, so we expect the code to try & establish a SPDY connection
		// which has been hijacked & blocked by the scaffold
		require.Error(t, err)
		require.Contains(t, err.Error(), "SPDY connections blocked")
		require.Nil(t, result)
	})
}

func TestClientExecWrongServiceName(t *testing.T) {
	withExecTestScaffold(t, nil, func(s *execScaffold) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		_, err := s.client.Exec(s.ctx,
			s.leaseID,
			"nottheservice",
			0,
			[]string{"/bin/true"},
			nil,
			stdout,
			stderr,
			false,
			nil)
		require.ErrorIs(t, err, cluster.ErrExecNoServiceWithName)
	})
}

func TestClientExecWrongPodIndex(t *testing.T) {
	withExecTestScaffold(t, nil, func(s *execScaffold) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		_, err := s.client.Exec(s.ctx,
			s.leaseID,
			execTestServiceName,
			9999,
			[]string{"/bin/true"},
			nil,
			stdout,
			stderr,
			false,
			nil)
		require.ErrorIs(t, err, cluster.ErrExecPodIndexOutOfRange)
	})
}

func TestClientExecPodNotRunning(t *testing.T) {
	withExecTestScaffold(t, func(pod *corev1.Pod) error {
		pod.Status.Phase = corev1.PodSucceeded
		return nil
	}, func(s *execScaffold) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		_, err := s.client.Exec(s.ctx,
			s.leaseID,
			execTestServiceName,
			0,
			[]string{"/bin/true"},
			nil,
			stdout,
			stderr,
			false,
			nil)
		require.ErrorIs(t, err, cluster.ErrExecServiceNotRunning)
		require.Contains(t, err.Error(), "service has completed")
	})
}

func TestClientExecPodFailed(t *testing.T) {
	withExecTestScaffold(t, func(pod *corev1.Pod) error {
		pod.Status.Phase = corev1.PodFailed
		return nil
	}, func(s *execScaffold) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		_, err := s.client.Exec(s.ctx,
			s.leaseID,
			execTestServiceName,
			0,
			[]string{"/bin/true"},
			nil,
			stdout,
			stderr,
			false,
			nil)
		require.ErrorIs(t, err, cluster.ErrExecServiceNotRunning)
		require.Contains(t, err.Error(), "service has failed")
	})
}

func TestClientExecPodNotReady(t *testing.T) {
	withExecTestScaffold(t, func(pod *corev1.Pod) error {
		pod.Status.Conditions[0].Status = corev1.ConditionFalse
		return nil
	}, func(s *execScaffold) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		_, err := s.client.Exec(s.ctx,
			s.leaseID,
			execTestServiceName,
			0,
			[]string{"/bin/true"},
			nil,
			stdout,
			stderr,
			false,
			nil)
		require.ErrorIs(t, err, cluster.ErrExecServiceNotRunning)
		require.Contains(t, err.Error(), "service is not ready")
	})
}
