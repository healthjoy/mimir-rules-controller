# Mimir Rules Controller

Kubernetes controller for managing [Mimir](github.com/grafana/mimir) rules as Kubernetes resources.
This controller heavily inspired by [Prometheus Operator](github.com/coreos/prometheus-operator)
and [Thanos Rule Controller](github.com/thanos-io/thanos/tree/master/pkg/operator/rule).

## Installation

Use the package manager [helm](https://helm.sh/) to install Mimir Rules Controller. You need to specify the address of your mimir installation and cluster name.

```bash
helm install --set mimir.address=mimir.example.com --set mimir.clusterName=example-cluster mimir-rules-controller ./deployments/charts/mimir-rules-controller 
```

That will install CRDs and the controller itself.

## Usage

To add rules or alerts for Mimir using the controller, you need to create a custom resource in a cluster with the installed controller:

```yaml
apiVersion: rulescontroller.k8s.healthjoy.com/v1alpha1
kind: MimirRule
metadata:
    name: example-mimirrule
    namespace: default
spec:
    groups:
    - name: example-mimirrule
      rules:
      - alert: example-mimirrule
        expr: 1
        for: 1m
        labels:
            severity: critical
        annotations:
            summary: example-mimirrule
            description: example-mimirrule
```

## Contributing

Pull requests are welcome. For major changes, please open an issue first
to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License

[MIT](https://choosealicense.com/licenses/mit/)
