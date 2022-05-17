# `k8s-charter`

Plots CPU and memory usage metrics charts based on your running pods.

## Prequisites

All commands here are based off Linux shell. Other operating systems should be similar, especially
MacOS. For Windows, the executable will have file extension `.exe`.

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

## How to run

You will need to build the executable first (follow the guide above), or simply download from
<https://github.com/dsaidgovsg/k8s-charter/releases>.

```bash
# Remember to create your config.yaml as shown above
./k8s-charter
```

You should see logs in your terminal, while the `.html` and `.json` files are being generated based
on your configured output file name (or updated) on every tick.

You can simply open up the `.html` file to see the chart.

## How to read the chart

The charts are zoomed based on the received values for CPU and Memory usage from the metrics server,
and is not limited to the Request values for CPU and Memory based on the Deployment.

The X-axis represents the tick, where one tick is equivalent to the `interval` value set in
`config.yaml`.

The Y-axis represents the sum of CPU / Memory values across all grouped pods.

The line plotted on every tick on the chart is the sum value of CPU / Memory at that tick.

The values found below the title of the chart consist of the following:

- `min` and `max` values are the min and max sum values of CPU / Memory across all ticks
- `avg` value is the sum of average values in every tick, divided by the number of ticks. The
  average value in every tick is the sum of all values in that tick, divided by the number of pods
  at that tick.
- `k8s request` is the CPU / Memory Request value from the Kuberetes Deployment for the pod.

All percentage values shown are the values divided by the Request value.
