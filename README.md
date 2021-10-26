# kubectl-mount

A kubectl plugin to mount remote filesystem on Kubernetes pods

## :hammer_and_wrench: Developing

Create a cluster:

```console
$ kind create cluster --config .kind/cluster.yaml
```

Then deploy nginx with a Deployment and PodDIsruptionBudget:

```console
$ kubectl apply -f .kind/deployment.yaml
```

## :memo: LICENSE

[MIT](./LICENSE)
