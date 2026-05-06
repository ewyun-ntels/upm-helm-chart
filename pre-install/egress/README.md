# EgressIP CRD Pre-install

Apply this manifest before installing the tcp-bridge chart with
`egressIP.enabled=true`.

```bash
kubectl apply -f pre-install/egress/egressip-crd.yaml
kubectl get crd egressips.k8s.ovn.org
```

This CRD only registers the `EgressIP` API type. Actual source IP NAT requires
OVN-Kubernetes EgressIP controller support and assignable egress nodes.
