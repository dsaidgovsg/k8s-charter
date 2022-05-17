package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/viper"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/templates"
)

func wrap(values []int64) []opts.LineData {
	items := make([]opts.LineData, 0)

	for i := 0; i < len(values); i++ {
		items = append(items, opts.LineData{Value: values[i]})
	}
	return items
}

func wrapFloats(values []float64) []opts.LineData {
	items := make([]opts.LineData, 0)

	for i := 0; i < len(values); i++ {
		items = append(items, opts.LineData{Value: values[i]})
	}
	return items
}

func avgPctOverReq(value int64, pod int64, req int64) float64 {
	return float64(value) / float64(pod) / float64(req) * 100
}

func avgPctsOverReq(values []int64, pods []int64, req int64) []float64 {
	avgs := make([]float64, len(values))

	// values and pods must have the same length
	for i, value := range values {
		pod := pods[i]
		avgs[i] = avgPctOverReq(value, pod, req)
	}

	return avgs
}

/// Returns (min, max, avg)
func summarize(values []int64) (int64, int64, int64) {
	minV := int64(math.MaxInt64)
	maxV := int64(math.MinInt64)
	total := int64(0)

	for _, v := range values {
		total += v
		minV = min(minV, v)
		maxV = max(maxV, v)
	}

	avgV := total / int64(len(values))

	return minV, maxV, avgV
}

func summarizeFloats(values []float64) (float64, float64, float64) {
	minV := float64(math.MaxFloat64)
	maxV := float64(-math.MaxFloat64)
	total := float64(0)

	for _, v := range values {
		total += v
		minV = minFloat(minV, v)
		maxV = maxFloat(maxV, v)
	}

	avgV := total / float64(len(values))

	return minV, maxV, avgV
}

func min(x int64, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

func minFloat(x float64, y float64) float64 {
	if x < y {
		return x
	}
	return y
}

func max(x int64, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

func maxFloat(x float64, y float64) float64 {
	if x > y {
		return x
	}
	return y
}

func generateDefaultYAxisOpts(name string) opts.YAxis {
	return opts.YAxis{
		Name: name,
		SplitLine: &opts.SplitLine{
			Show: false,
		},
		AxisLabel: &opts.AxisLabel{
			Inside: true,
		},
	}
}

func generateChartsOpts(others ...charts.GlobalOpts) []charts.GlobalOpts {
	return append(
		[]charts.GlobalOpts{
			charts.WithXAxisOpts(opts.XAxis{
				Name: "tick",
				Show: true,
			}),
			// Y-Axis currently overlaps with the Subtitle https://github.com/go-echarts/go-echarts/issues/233
			charts.WithYAxisOpts(generateDefaultYAxisOpts("")),
			charts.WithTooltipOpts(opts.Tooltip{
				Show:      true,
				TriggerOn: "mousemove",
			}),
		},
		others...,
	)
}

func overridePageTpl() {
	// Every chart element is placed into a div with `container` class
	templates.PageTpl = `
{{- define "page" }}
<!DOCTYPE html>
<html>
    {{- template "header" . }}
<body>
    <style> .container {float: left; width: 50%;} .item {margin: auto;} </style>
    {{- range .Charts }} {{ template "base" . }} {{- end }}
</body>
</html>
{{ end }}
`
}

type reqUsage struct {
	cpu int64
	mem int64
}

type Usage struct {
	Cpus       []int64
	Mems       []int64
	Pods       []int64
	RequestCpu int64
	RequestMem int64
	StartTime  time.Time
	Tick       int
}

func main() {
	overridePageTpl()

	// Read application config
	viper.SetConfigName("config")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %w \n", err))
	}

	namespace := viper.GetString("namespace") // Set to "" for all namespaces
	interval := viper.GetInt("interval")
	groups := viper.GetStringSlice("groups")
	maxTicks := viper.GetInt("maxTicks") // Set to -1 for running to forever
	if maxTicks < 0 {
		maxTicks = math.MaxInt
	}
	htmlOutputPath := viper.GetString("htmlOutputPath")
	jsonOutputPath := viper.GetString("jsonOutputPath")

	// Set up chart

	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	dpys, err := clientset.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})

	if err != nil {
		panic(err)
	}

	requestUsages := make(map[string]*reqUsage)

	for _, dpy := range dpys.Items {
		for _, group := range groups {
			if dpy.Labels["app.kubernetes.io/name"] == group {
				requests := dpy.Spec.Template.Spec.Containers[0].Resources.Requests

				requestUsages[group] = &reqUsage{}
				requestUsages[group].cpu = requests.Cpu().ScaledValue(resource.Milli)
				requestUsages[group].mem = requests.Memory().ScaledValue(resource.Mega)
			}
		}
	}

	mc, err := metrics.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// Setting up signal for graceful termination
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT)

	go func() {
		s := <-done
		switch s {
		case syscall.SIGINT:
			os.Exit(0)
		default:
			os.Exit(1)
		}
	}()

	// Hold data throughout time
	dateFmt := "2006-02-01T15:04:05"
	startTime := time.Now()
	startTimeStr := fmt.Sprintf("%v", startTime.Format(dateFmt))

	// Inject datetime value to output path
	dateTemplate := "{{date}}"
	htmlOutputPath = strings.ReplaceAll(htmlOutputPath, dateTemplate, startTimeStr)
	jsonOutputPath = strings.ReplaceAll(jsonOutputPath, dateTemplate, startTimeStr)

	tick := 0
	var tickSeries []int

	groupUsages := make(map[string]*Usage)

	for tick < maxTicks {
		maxTicksStr := fmt.Sprintf("%v", maxTicks)
		if maxTicks == math.MaxInt {
			maxTicksStr = "MAX"
		}
		fmt.Printf("%v (tick %v/%v)\n", time.Now().Format(dateFmt), tick+1, maxTicksStr)

		tickSeries = append(tickSeries, tick)

		pms, err := mc.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err)
		}

		// To hold all charts
		overridePageTpl() // This renders page.SetLayout useless because we are overriding the HTML template that uses this value
		page := components.NewPage()

		// Set HTML title based on output file name
		outputPathParts := strings.Split(htmlOutputPath, "/")
		page.PageTitle = outputPathParts[len(outputPathParts)-1]

		for _, group := range groups {
			matchingPods := int64(0)
			totalCpu := int64(0)
			totalMem := int64(0)

			for _, pm := range pms.Items {
				for _, container := range pm.Containers {
					if container.Name == group {
						matchingPods++

						cpu := container.Usage.Cpu().ScaledValue(resource.Milli)
						mem := container.Usage.Memory().ScaledValue(resource.Mega)

						totalCpu += cpu
						totalMem += mem

						fmt.Printf("%v:  CPU: %vm  MEM: %vMi\n", container.Name, cpu, mem)
					}
				}
			}

			if matchingPods > 0 {
				requestUsage := requestUsages[group]

				if _, ok := groupUsages[group]; !ok {
					groupUsages[group] = &Usage{}
					groupUsage := groupUsages[group]

					groupUsage.RequestCpu = requestUsage.cpu
					groupUsage.RequestMem = requestUsage.mem
					groupUsage.StartTime = startTime
					groupUsage.Tick = interval
				}

				groupUsage := groupUsages[group]
				groupUsage.Cpus = append(groupUsage.Cpus, totalCpu)
				groupUsage.Mems = append(groupUsage.Mems, totalMem)
				groupUsage.Pods = append(groupUsage.Pods, matchingPods)

				minCpu, maxCpu, avgCpu := summarize(groupUsage.Cpus)
				minMem, maxMem, avgMem := summarize(groupUsage.Mems)

				// Averaging across the group
				subtitleFmt := "[pods: %v, start time: %v, tick: %vs] min: %v, max: %v, avg: %v, k8s request per pod: %v"
				subtitlePctFmt := "[pods: %v, start time: %v, tick: %vs] min: %.2f%%, max: %.2f%%, avg: %.2f%%"

				// Format pod count
				minPods, maxPods, _ := summarize(groupUsage.Pods)
				podsStr := fmt.Sprintf("%v", minPods)
				if minPods != maxPods {
					podsStr = fmt.Sprintf("%v-%v", minPods, maxPods)
				}

				podsWrap := wrap(groupUsage.Pods)

				// Total CPU chart generation
				cpuLine := charts.NewLine()
				cpuSubtitle := fmt.Sprintf(subtitleFmt, podsStr, startTimeStr, interval,
					minCpu, maxCpu, avgCpu, groupUsage.RequestCpu)

				cpuLine.SetGlobalOptions(
					generateChartsOpts(
						charts.WithTitleOpts(opts.Title{
							Title:    fmt.Sprintf("%v (Total CPU [m])", group),
							Subtitle: cpuSubtitle,
						}),
					)...,
				)
				cpuLine.ExtendYAxis(generateDefaultYAxisOpts("Pods"))

				cpuLine.SetXAxis(tickSeries).
					AddSeries("Memory", wrap(groupUsage.Cpus), charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 0})).
					AddSeries("Pods", podsWrap, charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 1}))

				// CPU percentage chart generation
				cpuPcts := avgPctsOverReq(groupUsage.Cpus, groupUsage.Pods, groupUsage.RequestCpu)
				minCpuPct, maxCpuPct, avgCpuPct := summarizeFloats(cpuPcts)

				cpuPctsLine := charts.NewLine()
				cpuPctsSubtitle := fmt.Sprintf(subtitlePctFmt, podsStr, startTimeStr, interval,
					minCpuPct, maxCpuPct, avgCpuPct)

				cpuPctsLine.SetGlobalOptions(
					generateChartsOpts(
						charts.WithTitleOpts(opts.Title{
							Title:    fmt.Sprintf("%v (CPU Average [%%])", group),
							Subtitle: cpuPctsSubtitle,
						}),
					)...,
				)
				cpuPctsLine.ExtendYAxis(generateDefaultYAxisOpts("Pods"))

				cpuPctsLine.SetXAxis(tickSeries).
					AddSeries("CPU %", wrapFloats(cpuPcts), charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 0})).
					AddSeries("Pods", podsWrap, charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 1}))

				// Total Memory chart generation
				memLine := charts.NewLine()
				memSubtitle := fmt.Sprintf(subtitleFmt, podsStr, startTimeStr, interval,
					minMem, maxMem, avgMem, groupUsage.RequestMem)

				memLine.SetGlobalOptions(
					generateChartsOpts(
						charts.WithTitleOpts(opts.Title{
							Title:    fmt.Sprintf("%v (Total Memory [Mi])", group),
							Subtitle: memSubtitle,
						}),
					)...,
				)
				memLine.ExtendYAxis(generateDefaultYAxisOpts("Pods"))

				memLine.SetXAxis(tickSeries).
					AddSeries("Memory", wrap(groupUsage.Mems), charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 0})).
					AddSeries("Pods", podsWrap, charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 1}))

				// Memory percentage chart generation
				memPcts := avgPctsOverReq(groupUsage.Mems, groupUsage.Pods, groupUsage.RequestMem)
				minMemPct, maxMemPct, avgMemPct := summarizeFloats(memPcts)

				memPctsLine := charts.NewLine()
				memPctsSubtitle := fmt.Sprintf(subtitlePctFmt, podsStr, startTimeStr, interval,
					minMemPct, maxMemPct, avgMemPct)

				memPctsLine.SetGlobalOptions(
					generateChartsOpts(
						charts.WithTitleOpts(opts.Title{
							Title:    fmt.Sprintf("%v (Memory Average [%%])", group),
							Subtitle: memPctsSubtitle,
						}),
					)...,
				)
				memPctsLine.ExtendYAxis(generateDefaultYAxisOpts("Pods"))

				memPctsLine.SetXAxis(tickSeries).
					AddSeries("Memory %", wrapFloats(memPcts), charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 0})).
					AddSeries("Pods", podsWrap, charts.WithLineChartOpts(opts.LineChart{YAxisIndex: 1}))

				page.AddCharts(cpuLine).AddCharts(cpuPctsLine).
					AddCharts(memLine).AddCharts(memPctsLine)
			}
		}

		// For verbose sanity check if no matches
		if len(groupUsages) == 0 {
			fmt.Println("No matching deployment name found!")
		}

		// Write out the HTML
		f, _ := os.Create(htmlOutputPath)
		page.Render(io.MultiWriter(f))

		// Write out the JSON representation (useful for reading back in future into charts)
		jsonData, err := json.MarshalIndent(groupUsages, "", "  ")
		if err != nil {
			panic(err)
		}

		err = ioutil.WriteFile(jsonOutputPath, jsonData, 0755)
		if err != nil {
			panic(err)
		}

		time.Sleep(time.Duration(interval) * time.Second)
		tick++
	}
}
