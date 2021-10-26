package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	mountUsageStr = "mount [user@]pod:[dir] mountpoint"

	mountExample = `
	# Mount a pod nginx
	kubectl mount nginx:/etc /tmp/nginx/etc`
)

type MountOptions struct {
	configFlags *genericclioptions.ConfigFlags

	User       string
	PodName    string
	RemoteDir  string
	MountPoint string

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
	return nil
}

// Run mounts a pod or pods on the resources
func (o *MountOptions) RunMount(ctx context.Context) error {
	fmt.Printf("would to mount %s@%s:%s to %q\n",
		o.User, o.PodName, o.RemoteDir, o.MountPoint)
	return nil
}
