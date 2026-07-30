// Package dockerutil provides Docker container management utilities
package dockerutil

import (
	"os/exec"
	"strings"
)


// ServiceStatus handles the ServiceStatus HTTP request.
func ServiceStatus() (smpStatus, xftpStatus string) {
	smpStatus = "unknown"
	xftpStatus = "unknown"
	if b, err := exec.Command("docker", "ps", "--filter", "name=ParanoidX-smp-server", "--format", "{{.Status}}").Output(); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" {
			smpStatus = s
		}
	}
	if b, err := exec.Command("docker", "ps", "--filter", "name=ParanoidX-xftp-server", "--format", "{{.Status}}").Output(); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" {
			xftpStatus = s
		}
	}
	return
}


// ApplyStatus handles the ApplyStatus HTTP request.
func ApplyStatus(smpStatus, xftpStatus *string, svc map[string]any) {
	if name, ok := svc["name"].(string); ok {
		switch name {
		case "smp":
			*smpStatus = "running"
		case "xftp":
			*xftpStatus = "running"
		}
	}
}
