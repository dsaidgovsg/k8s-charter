# `k8s-charter`

[![CI Status](https://img.shields.io/github/workflow/status/dsaidgovsg/k8s-charter/ci/master?label=ci&logo=github&style=for-the-badge)](https://github.com/dsaidgovsg/k8s-charter/actions)

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
  maxTicks: 20  # Max number of polls before stopping the program, set -1 to run forever (CTRL-C to break)
  htmlOutputPath: "k8s-charter-{{date}}.html"  # html output, {{date}} to inject in datetime value
  jsonOutputPath: "k8s-charter-{{date}}.json"  # json output, {{date}} to inject in datetime value
  ```

## How to install

### Method 1 - Download from release assets (recommended)

Prebuilt statically linked binaries are released for every tag (and also every `master` commit in
`nightly` tag if you do not mind latest working release).

Head over to <https://github.com/dsaidgovsg/k8s-charter/releases> to get the binaries.

### Method 2 - `go install`

If you have `go` binary and do not mind compilation, you can also do

```bash
go install github.com/dsaidgovsg/k8s-charter@v1.0.0   # Or change to any other tagged version
go install github.com/dsaidgovsg/k8s-charter@latest   # For latest binary
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

Optionally, if you want to override the application version `-version` value, you can do

```bash
go build -ldflags "-X main.appVersion=yourversion"
./k8s-charter -version  # Should show "yourversion"
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

Every valid group in `groups` based on `config.yaml` will generate 4 charts:

- Total CPU (millicores)
- CPU Average (%)
- Total Memory (Mi)
- Memory Average (%)

For the non-percentage based charts, the values are always the sum of all millicores (or memory).

For the percentage based charts, the values are always the sum of all millicores (or memory),
divided by (requested millicores or memory * number of running pods at that tick). It is possible
to exceed 100% because the Kubernetes Request is not the Limit value.

For every chart, there is one X-axis and two Y-axis series:

- X-axis represents the tick, where one tick is equivalent to the `interval` value set in
`config.yaml`.
- Primary Y-axis represents the total value, or average percentage based on the chart type as
  described above.
- Secondary Y-axis represents the pod count, which can vary if the cluster has Horizontal Pod
  Autoscaler (or any other form of autoscaling).

In addition, each chart comes with the min, max, and the k8s request values.

If the reading at a particular tick is not obvious, you may mouseover on any dot on the chart; it
should reveal the exact value in the form of `(tick): (value)` format.
