package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
)

const (
	mountUsageStr = "mount [user@]pod:[dir] mountpoint"

	mountExample = `
	# Mount a remote filesystem of default container on the pod nginx
	kubectl mount nginx:/etc /tmp/nginx/etc

	# Mount a remote filesystem of side-car container on the pod nginx
	kubectl mount -c sidecar nginx:/etc /tmp/sidecar/etc`
)

type MountOptions struct {
	configFlags *genericclioptions.ConfigFlags

	User          string
	PodName       string
	RemoteDir     string
	MountPoint    string
	Namespace     string
	ContainerName string
	Debug         bool

	genericclioptions.IOStreams
}

func NewMountOptions(streams genericclioptions.IOStreams) *MountOptions {
	configFlags := genericclioptions.NewConfigFlags(true)

	return &MountOptions{
		configFlags: configFlags,

		IOStreams: streams,
	}
}

// NewCmdMount provides a cobra command wrapping MountOptions
func NewCmdMount(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewMountOptions(streams)

	cmd := &cobra.Command{
		Use:          mountUsageStr,
		Short:        "Mount a remote filesystem on the pods",
		Example:      mountExample,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.RunMount(c.Context()); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&o.ContainerName, "container", "c", "", "Container name. If omitted, use the kubectl.kubernetes.io/default-container annotation for selecting the container to be attached or the first container in the pod will be chosen")
	cmd.Flags().BoolVar(&o.Debug, "debug", false, "Print fuse debug log if true")
	o.configFlags.AddFlags(cmd.PersistentFlags())

	return cmd
}

func (o *MountOptions) Complete(c *cobra.Command, args []string) error {
	if len(args) != 2 {
		return errors.New("remote filesystem and mountpoint is required")
	}

	remote, mountpoint := args[0], args[1]
	o.MountPoint = mountpoint

	if !strings.Contains(remote, ":") {
		return fmt.Errorf("expected '%s'. The remote filesystem should contain ':'", mountUsageStr)
	}

	sp := strings.Split(remote, ":")
	o.RemoteDir = sp[1]

	if strings.Contains(sp[0], "@") {
		sp := strings.Split(sp[0], "@")
		o.User = sp[0]
		o.PodName = sp[1]
		if o.User == "" {
			return fmt.Errorf("expected '%s'. The remote user name is empty", mountUsageStr)
		}
	} else {
		o.PodName = sp[0]
	}
	if o.PodName == "" {
		return fmt.Errorf("expected '%s'. The pod name is empty", mountUsageStr)
	}

	namespace, _, err := o.configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}
	o.Namespace = namespace

	return nil
}

// Run mounts a pod or pods on the resources
func (o *MountOptions) RunMount(ctx context.Context) error {
	clientConfig, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return err
	}
	err = setKubernetesDefaults(clientConfig)
	if err != nil {
		return err
	}

	api, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}
	pod, err := api.CoreV1().Pods(o.Namespace).Get(ctx, o.PodName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return fmt.Errorf("cannot mount filesystem on the container in a completed pod; current phase is %s", pod.Status.Phase)
	}

	containerName := o.ContainerName
	if containerName == "" {
		containerName = pod.Spec.Containers[0].Name
	}

	restClient, err := restclient.RESTClientFor(clientConfig)
	if err != nil {
		return err
	}

	e := &PodExecutor{
		Namespace:     pod.GetNamespace(),
		PodName:       pod.GetName(),
		ContainerName: containerName,
		Config:        clientConfig,
		RestClient:    restClient,
	}

	fsys := &PodFS{
		Executor: e,
		Pwd:      o.RemoteDir,
	}
	root := &PodFuseNode{
		fsys: fsys,
	}

	var opt fusefs.Options
	opt.Debug = o.Debug
	srv, err := fusefs.Mount(o.MountPoint, root, &opt)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		<-ch
		err = srv.Unmount()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Unable to unmount:", err)
		}
	}()
	fmt.Fprintf(os.Stderr, "Mounted %s:%s on %s\n", o.PodName, o.RemoteDir, o.MountPoint)
	srv.Wait()

	return nil
}

// See https://github.com/kubernetes/kubernetes/blob/10988997f225447f89841bac08e8848852d7cb55/staging/src/k8s.io/kubectl/pkg/cmd/util/kubectl_match_version.go#L115
func setKubernetesDefaults(config *restclient.Config) error {
	config.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"}
	if config.APIPath == "" {
		config.APIPath = "/api"
	}
	if config.NegotiatedSerializer == nil {
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}
	return restclient.SetKubernetesDefaults(config)
}
