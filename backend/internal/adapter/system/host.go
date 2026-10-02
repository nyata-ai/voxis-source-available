package system

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/voxis/backend/internal/port"
)

// hostCollector reads host vitals from /proc and statfs. All sources are
// injectable for testing.
type hostCollector struct {
	procRoot     string
	numCPU       func() int
	processStart time.Time
	now          func() time.Time
	statfs       func(path string, buf *syscall.Statfs_t) error
}

func newHostCollector(processStart time.Time, now func() time.Time) *hostCollector {
	return &hostCollector{
		procRoot:     "/proc",
		numCPU:       runtime.NumCPU,
		processStart: processStart,
		now:          now,
		statfs:       syscall.Statfs,
	}
}

// u64ToI64 converts a uint64 to int64, clamping at math.MaxInt64 to keep the
// conversion overflow-safe (statfs block counts are physically far below this).
func u64ToI64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

func (h *hostCollector) readProc(name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(h.procRoot, name)) //nolint:gosec // G304: procRoot is a fixed trusted path; name is always a hardcoded literal (meminfo/loadavg/uptime)
	if err != nil {
		return "", fmt.Errorf("read /proc/%s: %w", name, err)
	}
	return string(b), nil
}

func (h *hostCollector) memory() (port.SystemMemory, error) {
	body, err := h.readProc("meminfo")
	if err != nil {
		return port.SystemMemory{}, err
	}
	var totalKB, availKB int64
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		v, convErr := strconv.ParseInt(fields[1], 10, 64)
		if convErr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	if totalKB == 0 {
		return port.SystemMemory{}, fmt.Errorf("meminfo: MemTotal not found")
	}
	total := totalKB * 1024
	avail := availKB * 1024
	return port.SystemMemory{TotalBytes: total, AvailableBytes: avail, UsedBytes: total - avail}, nil
}

func (h *hostCollector) cpu() (port.SystemCPU, error) {
	body, err := h.readProc("loadavg")
	if err != nil {
		return port.SystemCPU{}, err
	}
	fields := strings.Fields(body)
	if len(fields) < 3 {
		return port.SystemCPU{}, fmt.Errorf("loadavg: unexpected format")
	}
	l1, err1 := strconv.ParseFloat(fields[0], 64)
	l5, err5 := strconv.ParseFloat(fields[1], 64)
	l15, err15 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err5 != nil || err15 != nil {
		return port.SystemCPU{}, fmt.Errorf("loadavg: parse error")
	}
	return port.SystemCPU{Load1: l1, Load5: l5, Load15: l15, Cores: h.numCPU()}, nil
}

func (h *hostCollector) uptime() (port.SystemUptime, error) {
	body, err := h.readProc("uptime")
	if err != nil {
		return port.SystemUptime{}, err
	}
	fields := strings.Fields(body)
	if len(fields) < 1 {
		return port.SystemUptime{}, fmt.Errorf("uptime: unexpected format")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return port.SystemUptime{}, fmt.Errorf("uptime: parse error: %w", err)
	}
	proc := int64(h.now().Sub(h.processStart).Seconds())
	return port.SystemUptime{HostSeconds: int64(secs), ProcessSeconds: proc}, nil
}

func (h *hostCollector) disk(mount string) (port.SystemDiskStats, error) {
	var buf syscall.Statfs_t
	if err := h.statfs(mount, &buf); err != nil {
		return port.SystemDiskStats{}, fmt.Errorf("statfs %s: %w", mount, err)
	}
	bsize := buf.Bsize
	total := u64ToI64(buf.Blocks) * bsize
	free := u64ToI64(buf.Bavail) * bsize
	used := (u64ToI64(buf.Blocks) - u64ToI64(buf.Bfree)) * bsize
	return port.SystemDiskStats{
		Available: true, Mount: mount,
		TotalBytes: total, FreeBytes: free, UsedBytes: used,
	}, nil
}
