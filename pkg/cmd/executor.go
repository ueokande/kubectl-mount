package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

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
	RunRead(ctx context.Context, command []string) (io.ReadCloser, error)
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

type bufferCloser struct {
	bytes.Buffer
	closed bool
	ch     chan struct{}
}

func (b *bufferCloser) Done() <-chan struct{} {
	b.ch = make(chan struct{})
	return b.ch
}

func (b *bufferCloser) Close() error {
	if b.closed {
		return nil
	}
	b.closed = true
	b.ch <- struct{}{}
	return nil
}

func (e *PodExecutor) RunRead(ctx context.Context, command []string) (io.ReadCloser, error) {
	req := e.RestClient.Post().
		Resource("pods").
		Name(e.PodName).
		Namespace(e.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: e.ContainerName,
		Command:   command,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(e.Config, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	var stdout bufferCloser
	var stderr bytes.Buffer
	var stdin bytes.Buffer
	err = executor.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  &stdin,
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

	go func() {
		<-stdout.Done()
		stdin.Write([]byte{0x03}) // Ctrl-C
		io.Copy(io.Discard, &stdout)
	}()

	return &stdout, nil
}
