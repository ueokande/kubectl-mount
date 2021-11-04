# kubectl-mount

A kubectl plugin to mount a directory on Kubernetes pods to the local filesystem.

## :cd: Installation

```console
$ go install github.com/ueokande/kubectl-mount@latest
```
## :notebook_with_decorative_cover: Usage

Mount a log directory on the pod nginx to the local directory:

```console
$ mkdir -p /tmp/nginx-logs
$ kubectl mount nginx:/var/log /tmp/nginx-logs
```

The output is similar to this:

```
Mounted nginx:/var/log on /tmp/nginx-logs
```

The command runs until you interrupt the command (pressing <kbd>Ctrl</kbd>+<kbd>C</kbd>).  If the directory is during use and you exit the command, the command output the following error:

```
Unable to unmount: /bin/fusermount: failed to unmount /tmp/nginx-logs: Device or resource busy
```

That means the `kubectl mount` unable to unmount the filesystem.  This error can occur when any processes open the mounted file or directory.  That also cause when your shell enters the mounted directory.  To resolve this error, exit the process which is using the file or directory, and then type the following:

```console
$ fusermount -u /tmp/nginx
```

## :diving_mask: How does it work

The `kubectl mount` command works with the FUSE (Filesystem in Userspace) to mount a directory to the local filesystem.  The FUSE is an interface to userspace programs to export a filesystem to the kernel.  It allows showing users an interface to mount a variety of filesystems like a physical device, network storage, ramfs, and so on.  Users can implement it to create any programmable filesystem.  The [go-fuse][] is a library to implement a FUSE interface in golang.  It works on Linux with FUSE and macOS with OSXFUSE.

The `kubectl mount` provides a filesystem to show files in the Kubernetes pods.  It retrieve files or directories or read files in the pod via the Kubernetes `exec` API.  When you get the list in the directory, the `ls` command runs on the pod and returns files on the directory via FUSE.  Getting file information (creation time, modification time, owner, group) works with the result of the `stat` command.

```
                                                             +--------------+
                                                       cat   |  Kubernetes  |
                                                       stat  |+------------+|
        .--------------------. .--------------------.  ls    ||   NGINX    ||
        | ls /tmp/nginx-logs | |   kubectl-mount   <-----------> /var/log  ||
        '--------------------' '----------A---------'        ||            ||
        .---------|-----------------------|---------.        |+------------+|
        |         |      system call      |         |        +--------------+
User    '---------|-----------------------|---------'
==================|=======================|==================
Kernel  .---------|-----------------------|---------.
        |         '-------> FUSE ---------'         |
        '-------------------------------------------'
```
## :stop_sign: Limitation

The `kubectl mount` requires the following commands to be installed in the container:

- `find` or `ls`
- `stat`
- `cat`

The `kubectl mount` does not work well if the pod does not contain these commands, such as a container built from scratch.

## :hammer_and_wrench: Developing

Create a cluster:

```console
$ kind create cluster --config .kind/cluster.yaml
```

Then deploy deployment nginx:

```console
$ kubectl apply -f .kind/deployment.yaml
```

## :memo: LICENSE

[MIT](./LICENSE)

[go-fuse]: https://github.com/hanwen/go-fuse
