# Docker Exporter for Prometheus

This Go application exports Docker container metrics for Prometheus monitoring.

## Features

- Collects metrics such as CPU, memory, disk, and network usage for Docker containers.
- Exposes metrics in Prometheus format for monitoring and alerting.

## Prerequisites

- Go installed on your system ([Installation Guide](https://golang.org/doc/install))
- Docker installed and running on your system ([Installation Guide](https://docs.docker.com/get-docker/))

## Installation

1. Clone the repository:

```bash
git clone https://github.com/yourusername/docker-exporter.git
```

## build the application

1. cd docker-exporter
2. ``` go build ```

## Usage

1. run the exporter
```
./docker-exporter
```
The exporter will start collecting Docker container metrics and expose them on http://localhost:8815/metrics.

Add the Metrics endpoint to Prometheus
```
scrape_configs:
  - job_name: 'docker-exporter'
    static_configs:
      - targets: ['localhost:8815']
```

    Restart Prometheus to apply the configuration changes.

    Access Prometheus web UI (http://localhost:9090) and query the Docker Exporter metrics.

### Metrics

The Docker Exporter exposes the following metrics:

    container_status: Status of the Docker container (0 = created, 1 = running, 2 = stopped)
    disk_utilization: Disk utilization of the Docker container in kB/s
    memory_usage: Memory usage of the Docker container in MB
    network: Network usage of the Docker container in Mbps
    cpu_utilization: CPU utilization of the Docker container in %
    
