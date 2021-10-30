package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/exec"
)

type RemoteCommandErr struct {
	Stderr []byte
	Err    error
}

func (e *RemoteCommandErr) Error() string {
	return fmt.Sprintf("remote command error: %s", e.Stderr)
}

type Executor interface {
	Run(ctx context.Context, command []string) ([]byte, error)
}

type PodExecutor struct {
	Namespace     string
	PodName       string
	ContainerName string

	Config     *restclient.Config
	RestClient *restclient.RESTClient
}

func (e *PodExecutor) Run(ctx context.Context, command []string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	req := e.RestClient.Post().
		Resource("pods").
		Name(e.PodName).
		Namespace(e.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: e.ContainerName,
		Command:   command,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(e.Config, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	err = executor.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	var execerr exec.CodeExitError
	if errors.As(err, &execerr) {
		return nil, &RemoteCommandErr{
			Stderr: stderr.Bytes(),
			Err:    err,
		}
	}
	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}
