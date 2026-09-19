//go:build darwin

package collector

import (
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
)

type DarwinCollector struct{}

func New() SystemCollector {
	return &DarwinCollector{}
}

func (d *DarwinCollector) CollectHardware() (domain.SensorHardwareInfo, error) {
	hostname, _ := os.Hostname()
	cpuModel := getSysctlString("machdep.cpu.brand_string")
	if cpuModel == "" {
		cpuModel = getSysctlString("hw.model")
	}

	coresStr := getSysctlString("hw.ncpu")
	cores, _ := strconv.Atoi(coresStr)
	if cores <= 0 {
		cores = runtime.NumCPU()
	}

	memStr := getSysctlString("hw.memsize")
	totalMem, _ := strconv.ParseUint(memStr, 10, 64)

	serial := getDarwinSerialNumber()
	model := getSysctlString("hw.model")

	return domain.SensorHardwareInfo{
		SerialNumber:  serial,
		Hostname:      hostname,
		Manufacturer:  "Apple",
		Model:         model,
		Architecture:  runtime.GOARCH,
		CPUModel:      cpuModel,
		CPUCores:      cores,
		TotalRAMBytes: totalMem,
	}, nil
}

func (d *DarwinCollector) CollectOS() (domain.SensorOSInfo, error) {
	version := getSysctlString("kern.osproductversion")
	build := getSysctlString("kern.osversion")
	kernel := getSysctlString("kern.osrelease")

	return domain.SensorOSInfo{
		Platform: "darwin",
		Version:  version,
		Build:    build,
		Kernel:   kernel,
	}, nil
}

func (d *DarwinCollector) CollectState() (domain.SensorStateTelemetry, error) {
	var loggedInUser *string
	if u, err := user.Current(); err == nil && u != nil {
		name := u.Username
		if email := os.Getenv("USER_EMAIL"); email != "" {
			name = email
		}
		loggedInUser = &name
	}

	uptimeSec := getDarwinUptime()
	availRAM := getDarwinAvailableRAM()
	totalDisk, freeDisk := getDarwinDiskSpace("/")
	battPct, isAC := getDarwinBattery()

	var onAC *bool
	if isAC != nil {
		onAC = isAC
	}

	return domain.SensorStateTelemetry{
		LoggedInUser:      loggedInUser,
		UptimeSeconds:     uptimeSec,
		AvailableRAMBytes: availRAM,
		TotalDiskBytes:    totalDisk,
		FreeDiskBytes:     freeDisk,
		BatteryPercent:    battPct,
		OnACPower:         onAC,
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func getSysctlString(key string) string {
	out, err := exec.Command("sysctl", "-n", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getDarwinSerialNumber() string {
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	out, err := cmd.Output()
	if err != nil {
		return "UNKNOWN-MAC-SERIAL"
	}

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if strings.Contains(l, "IOPlatformSerialNumber") {
			parts := strings.Split(l, "=")
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				return strings.Trim(val, `"`)
			}
		}
	}
	return "UNKNOWN-MAC-SERIAL"
}

func getDarwinUptime() uint64 {
	cmd := exec.Command("sysctl", "-n", "kern.boottime")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	// Output format: { sec = 1716388421, usec = 837421 }
	str := string(out)
	secIdx := strings.Index(str, "sec = ")
	if secIdx != -1 {
		rest := str[secIdx+6:]
		endIdx := strings.Index(rest, ",")
		if endIdx != -1 {
			bootSec, _ := strconv.ParseInt(strings.TrimSpace(rest[:endIdx]), 10, 64)
			if bootSec > 0 {
				now := time.Now().Unix()
				if now > bootSec {
					return uint64(now - bootSec)
				}
			}
		}
	}
	return 0
}

func getDarwinAvailableRAM() uint64 {
	// Approximation via vm_stat page size * free pages
	pageSizeStr := getSysctlString("hw.pagesize")
	pageSize, _ := strconv.ParseUint(pageSizeStr, 10, 64)
	if pageSize == 0 {
		pageSize = 4096
	}

	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0
	}

	var freePages uint64
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if strings.Contains(l, "Pages free:") {
			parts := strings.Split(l, ":")
			if len(parts) == 2 {
				val := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
				freePages, _ = strconv.ParseUint(val, 10, 64)
			}
		}
	}
	return freePages * pageSize
}

func getDarwinDiskSpace(path string) (total uint64, free uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return total, free
}

func getDarwinBattery() (pct *int, onAC *bool) {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return nil, nil
	}

	str := string(out)
	isAC := strings.Contains(str, "AC Power")
	onAC = &isAC

	idx := strings.Index(str, "%")
	if idx != -1 {
		// find preceding digits
		start := idx - 1
		for start >= 0 && (str[start] >= '0' && str[start] <= '9') {
			start--
		}
		val, err := strconv.Atoi(str[start+1 : idx])
		if err == nil {
			pct = &val
		}
	}
	return pct, onAC
}
