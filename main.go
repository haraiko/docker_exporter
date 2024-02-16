package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	containerStatusDesc = prometheus.NewDesc(
		"container_status",
		"Status of the Docker container (0 = created, 1 = running, 2 = stopped)",
		[]string{"id", "name"}, nil,
	)

	diskUtilizationDesc = prometheus.NewDesc(
		"disk_utilization",
		"Disk utilization of the Docker container in kB/s",
		[]string{"id", "name"}, nil,
	)

	memoryUsageDesc = prometheus.NewDesc(
		"memory_usage",
		"Memory usage of the Docker container in MB",
		[]string{"id", "name"}, nil,
	)

	networkDesc = prometheus.NewDesc(
		"network",
		"Network usage of the Docker container in Mbps",
		[]string{"id", "name"}, nil,
	)

	cpuUtilizationDesc = prometheus.NewDesc(
		"cpu_utilization",
		"CPU utilization of the Docker container in %",
		[]string{"id", "name"}, nil,
	)
)

type Exporter struct {
	dockerClient *client.Client
}

func NewExporter() (*Exporter, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &Exporter{dockerClient: cli}, nil
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- containerStatusDesc
	ch <- diskUtilizationDesc
	ch <- memoryUsageDesc
	ch <- networkDesc
	ch <- cpuUtilizationDesc
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	containers, err := e.dockerClient.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Println("Error listing containers:", err)
		return
	}

	var wg sync.WaitGroup
	for _, container := range containers {
		wg.Add(1)
		go func(c types.Container) {
			defer wg.Done()
			e.collectContainerMetrics(ch, c)
		}(container)
	}
	wg.Wait()
}

func calculateCPUPercentUnix(previousCPU, v types.CPUStats) float64 {
    cpuDelta := float64(v.CPUUsage.TotalUsage - previousCPU.CPUUsage.TotalUsage)
    systemDelta := float64(v.SystemUsage - previousCPU.SystemUsage)

    if systemDelta > 0.0 && cpuDelta > 0.0 {
        cpuPercent := (cpuDelta / systemDelta) * float64(len(v.CPUUsage.PercpuUsage)) * 100.0
        return cpuPercent
    }
    return 0.0
}

func (e *Exporter) collectContainerMetrics(ch chan<- prometheus.Metric, container types.Container) {
    containerID := container.ID[:12]
    containerName := strings.TrimLeft(container.Names[0], "/")

    // Collect container status
    status := 0
    if container.State == "running" {
        status = 1
    } else if container.State == "exited" {
        status = 2
    }
    ch <- prometheus.MustNewConstMetric(containerStatusDesc, prometheus.GaugeValue, float64(status), containerID, containerName)

    // Use Docker stats for more metrics
    stats, err := e.dockerClient.ContainerStats(context.Background(), container.ID, false)
    if err != nil {
        log.Println("Error getting container stats:", err)
        return
    }
    defer stats.Body.Close()

    var statsData types.StatsJSON
    if err := json.NewDecoder(stats.Body).Decode(&statsData); err != nil {
        log.Println("Error decoding container stats:", err)
        return
    }

    // Collect disk utilization
    diskUsage := statsData.StorageStats.ReadSizeBytes + statsData.StorageStats.WriteSizeBytes
    ch <- prometheus.MustNewConstMetric(diskUtilizationDesc, prometheus.GaugeValue, float64(diskUsage)/1024, containerID, containerName)

    // Collect memory usage
    memUsage := float64(statsData.MemoryStats.Usage / (1024 * 1024))
    ch <- prometheus.MustNewConstMetric(memoryUsageDesc, prometheus.GaugeValue, memUsage, containerID, containerName)

    // Collect network usage
    networkUsage := float64(statsData.Networks["eth0"].RxBytes+statsData.Networks["eth0"].TxBytes) * 8 / (1024 * 1024)
    ch <- prometheus.MustNewConstMetric(networkDesc, prometheus.GaugeValue, networkUsage, containerID, containerName)

    // Collect CPU usage
    cpuUsage := calculateCPUPercentUnix(statsData.PreCPUStats, statsData.CPUStats)
    ch <- prometheus.MustNewConstMetric(cpuUtilizationDesc, prometheus.GaugeValue, cpuUsage, containerID, containerName)
}

func main() {
	exporter, err := NewExporter()
	if err != nil {
		log.Fatal("Error creating exporter:", err)
	}

	prometheus.MustRegister(exporter)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8815", nil))
}
