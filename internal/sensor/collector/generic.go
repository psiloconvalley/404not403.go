//go:build !darwin

package collector

import (
	"os"
	"runtime"

	"github.com/psiloconvalley/404not403/internal/domain"
)

type GenericCollector struct{}

func New() SystemCollector {
	return &GenericCollector{}
}

func (g *GenericCollector) CollectHardware() (domain.SensorHardwareInfo, error) {
	hostname, _ := os.Hostname()
	return domain.SensorHardwareInfo{
		SerialNumber:  "GENERIC-SERIAL-001",
		Hostname:      hostname,
		Manufacturer:  "Generic",
		Model:         "Generic Workstation",
		Architecture:  runtime.GOARCH,
		CPUModel:      runtime.GOARCH,
		CPUCores:      runtime.NumCPU(),
		TotalRAMBytes: 16 * 1024 * 1024 * 1024,
	}, nil
}

func (g *GenericCollector) CollectOS() (domain.SensorOSInfo, error) {
	return domain.SensorOSInfo{
		Platform: runtime.GOOS,
		Version:  "1.0.0",
		Build:    "1",
		Kernel:   "generic",
	}, nil
}

func (g *GenericCollector) CollectState() (domain.SensorStateTelemetry, error) {
	user := os.Getenv("USER")
	return domain.SensorStateTelemetry{
		LoggedInUser:      &user,
		UptimeSeconds:     3600,
		AvailableRAMBytes: 8 * 1024 * 1024 * 1024,
		TotalDiskBytes:    500 * 1024 * 1024 * 1024,
		FreeDiskBytes:     250 * 1024 * 1024 * 1024,
	}, nil
}
