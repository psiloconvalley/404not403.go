package collector

import (
	"github.com/psiloconvalley/404not403/internal/domain"
)

// SystemCollector is the interface implemented by platform-specific collectors.
type SystemCollector interface {
	CollectHardware() (domain.SensorHardwareInfo, error)
	CollectOS() (domain.SensorOSInfo, error)
	CollectState() (domain.SensorStateTelemetry, error)
}
