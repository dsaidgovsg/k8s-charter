# `k8s-charter`

Plots CPU and memory usage metrics charts based on your running pods.

## Prequisites

- You will need to have [`metrics-server`](https://kubernetes-sigs.github.io/metrics-server/)
  deployed. This will allow the following `metrics` API to work:
  `kubectl get --raw /apis/metrics.k8s.io/v1beta1/pods`
  - Installation guide via `helm` can be found
    [here](https://artifacthub.io/packages/helm/metrics-server/metrics-server).
  - The helm chart alternative from <https://artifacthub.io/packages/helm/bitnami/metrics-server>
    should also work, but this repository has not been tested on it.

- You will need to specify the names of the containers from the above `metrics` API finding, which
  you can get the list via

  ```bash
  kubectl get --raw /apis/metrics.k8s.io/v1beta1/pods | jq -r '.items[].containers[].name' | sort | uniq
  ```

- The deployment names associated to the above containers in the `metrics` API output must be the
  same as the names of the containers, which should generally be implicitly true by default, but
  this has not been verified.

  You can find out the deployment names via

  ```bash
  kubectl get deployments -n app -o json | jq -r '.items[].metadata.labels["app.kubernetes.io/name"]' | sort | uniq
  ```

- Provide `config.yaml` in the same directory as your running executable `k8s-charter`. It should look like this:

  ```yaml
  namespace: "app"  # Set to "" for all namespaces
  interval: 15  # Polling interval, 15 seconds is likely the default interval for metrics-server
  groups:  # List of container/deployment names to group by
    - "name_of_container_or_deployment_1"
    - "name_of_container_or_deployment_2"
  maxTicks: -1  # Max number of polls before stopping the program
  htmlOutputPath: "k8s-charter-{{date}}.html"  # html output, {{date}} to inject in datetime value
  jsonOutputPath: "k8s-charter-{{date}}.json"  # json output, {{date}} to inject in datetime value
  ```

## How to build

Assuming you have `go` set-up, this is simply just

```bash
go build
```

and the executable `k8s-charter` (or `k8s-charter.exe` for Windows) should be generated.

For a statically + smaller release build, you can do

```bash
CGO_ENABLED=0 go build -ldflags "-s -w"
```
