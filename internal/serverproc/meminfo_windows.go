//go:build windows

package serverproc

import (
	"encoding/csv"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MemoryUsage returns pid's current working-set memory in bytes, as
// reported by `tasklist` (the same figure Task Manager's "Memory" column
// shows), or ok=false if it couldn't be determined.
func MemoryUsage(pid int) (bytes int64, ok bool) {
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0, false
	}
	record, err := csv.NewReader(strings.NewReader(string(out))).Read()
	if err != nil || len(record) < 5 {
		return 0, false
	}
	// record[4] looks like "512,340 K" -- strip the unit and thousands separators.
	field := strings.TrimSuffix(strings.TrimSpace(record[4]), "K")
	field = strings.ReplaceAll(strings.TrimSpace(field), ",", "")
	kb, err := strconv.ParseInt(field, 10, 64)
	if err != nil {
		return 0, false
	}
	return kb * 1024, true
}
