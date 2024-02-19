package main

import (
    "context"
    "encoding/json"
    "log"
    "net/http"
    "strings"
    "sync"
    "os"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/client"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    containerStatusDesc = prometheus.NewDesc(
        "container_status",
        "Status of the Docker container (0 = created, 1 = running, 2 = stopped)",
        []string{"id", "name", "hostname"}, nil,
    )

    diskUtilizationDesc = prometheus.NewDesc(
        "disk_utilization",
        "Disk utilization of the Docker container in kB/s",
        []string{"id", "name", "hostname"}, nil,
    )

    memoryUsageDesc = prometheus.NewDesc(
        "memory_usage",
        "Memory usage of the Docker container in MB",
        []string{"id", "name", "hostname"}, nil,
    )

    networkDesc = prometheus.NewDesc(
        "network",
        "Network usage of the Docker container in Mbps",
        []string{"id", "name", "hostname"}, nil,
    )

    cpuUtilizationDesc = prometheus.NewDesc(
        "cpu_utilization",
        "CPU utilization of the Docker container in %",
        []string{"id", "name", "hostname"}, nil,
    )
)

type Exporter struct {
    dockerClient *client.Client
    hostname     string // Add hostname field
}

func NewExporter() (*Exporter, error) {
    // Get the hostname of the server
    hostname, err := os.Hostname()
    if err != nil {
        return nil, err
    }

    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        return nil, err
    }
    return &Exporter{
        dockerClient: cli,
        hostname:     hostname, // Add hostname to Exporter struct
    }, nil
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
    ch <- prometheus.MustNewConstMetric(containerStatusDesc, prometheus.GaugeValue, float64(status), containerID, containerName, e.hostname)

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


    // Collect disk utilization (in bytes)
    diskReadBytes := sumBlockIOBytes(statsData.BlkioStats.IoServiceBytesRecursive, "read")
    diskWriteBytes := sumBlockIOBytes(statsData.BlkioStats.IoServiceBytesRecursive, "write")
    totalDiskBytes := diskReadBytes + diskWriteBytes

    // Convert disk utilization to megabytes (MB)
    diskUsageMB := float64(totalDiskBytes) / (1024 * 1024)
    ch <- prometheus.MustNewConstMetric(diskUtilizationDesc, prometheus.GaugeValue, diskUsageMB, containerID, containerName, e.hostname)

    // Collect memory usage
    memUsage := float64(statsData.MemoryStats.Usage / (1024 * 1024))
    ch <- prometheus.MustNewConstMetric(memoryUsageDesc, prometheus.GaugeValue, memUsage, containerID, containerName, e.hostname)

    // Collect network usage (in bytes)
    networkRxBytes := statsData.Networks["eth0"].RxBytes
    networkTxBytes := statsData.Networks["eth0"].TxBytes
    totalNetworkBytes := networkRxBytes + networkTxBytes

    // Convert network usage to megabytes (MB)
    networkUsageMB := float64(totalNetworkBytes) / (1024 * 1024)
    ch <- prometheus.MustNewConstMetric(networkDesc, prometheus.GaugeValue, networkUsageMB, containerID, containerName, e.hostname)

    // Collect CPU usage
    cpuUsage := calculateCPUPercentUnix(statsData.PreCPUStats, statsData.CPUStats)
    ch <- prometheus.MustNewConstMetric(cpuUtilizationDesc, prometheus.GaugeValue, cpuUsage, containerID, containerName, e.hostname)
}

func sumBlockIOBytes(ioStats []types.BlkioStatEntry, op string) uint64 {
    var sum uint64
    for _, entry := range ioStats {
        if entry.Op == op {
            sum += entry.Value
        }
    }
    return sum
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

func main() {
    exporter, err := NewExporter()
    if err != nil {
        log.Fatal("Error creating exporter:", err)
    }

    prometheus.MustRegister(exporter)

    http.Handle("/metrics", promhttp.Handler())
    log.Fatal(http.ListenAndServe(":8815", nil))
}
