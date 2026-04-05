package relay

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

const (
	mdnsService = "_hex._tcp"
)

// Discovery manages mDNS service advertisement and Tailscale hostname detection.
type Discovery struct {
	dnssdCmd      *exec.Cmd // dns-sd -R process
	port          int
	token         string
	name          string
	tailscaleHost string
	tailscaleIP   string
}

// NewDiscovery creates a discovery manager for the given WS port and auth token.
// name is the display name for mDNS advertisement; if empty, defaults to the system hostname.
func NewDiscovery(port int, token string, name string) *Discovery {
	if name == "" {
		hostname, _ := os.Hostname()
		name = hostname
	}
	d := &Discovery{
		port:  port,
		token: token,
		name:  name,
	}
	d.detectTailscale()
	return d
}

// StartMDNS begins advertising the hex service via the system mDNSResponder.
// Uses dns-sd -R which goes through macOS's built-in Bonjour, ensuring proper
// multicast delivery that works through the firewall.
func (d *Discovery) StartMDNS() error {
	hostname, _ := os.Hostname()

	// Build TXT record key=value pairs as arguments to dns-sd -R
	txtArgs := []string{
		"-R", d.name,  // instance name
		mdnsService,             // service type
		"local",                 // domain
		fmt.Sprintf("%d", d.port), // port
		fmt.Sprintf("token=%s", d.token),
	}

	if d.tailscaleHost != "" {
		txtArgs = append(txtArgs, fmt.Sprintf("tailscale=%s", d.tailscaleHost))
	}
	if d.tailscaleIP != "" {
		txtArgs = append(txtArgs, fmt.Sprintf("tsip=%s", d.tailscaleIP))
	}

	d.dnssdCmd = exec.Command("dns-sd", txtArgs...)
	d.dnssdCmd.Stdout = nil
	d.dnssdCmd.Stderr = nil

	if err := d.dnssdCmd.Start(); err != nil {
		return fmt.Errorf("dns-sd -R: %w", err)
	}

	log.Printf("[discovery] mDNS advertising %s on port %d via system mDNSResponder (host: %s, pid: %d)",
		mdnsService, d.port, hostname, d.dnssdCmd.Process.Pid)
	return nil
}

// detectTailscale checks if Tailscale is running and gets the hostname + IP.
func (d *Discovery) detectTailscale() {
	if err := exec.Command("tailscale", "status", "--peers=false").Run(); err != nil {
		return
	}

	if out, err := exec.Command("tailscale", "ip", "-4").Output(); err == nil {
		d.tailscaleIP = strings.TrimSpace(string(out))
	}

	if out, err := exec.Command("tailscale", "status", "--json").Output(); err == nil {
		if idx := strings.Index(string(out), `"DNSName":"`); idx >= 0 {
			rest := string(out)[idx+len(`"DNSName":"`):]
			if end := strings.Index(rest, `"`); end >= 0 {
				dnsName := strings.TrimSuffix(rest[:end], ".")
				if dnsName != "" {
					d.tailscaleHost = dnsName
				}
			}
		}
	}

	if d.tailscaleHost != "" {
		log.Printf("[discovery] Tailscale detected: %s (%s)", d.tailscaleHost, d.tailscaleIP)
	} else if d.tailscaleIP != "" {
		log.Printf("[discovery] Tailscale IP: %s (no hostname)", d.tailscaleIP)
	}
}

// TailscaleHost returns the Tailscale FQDN, or empty if not available.
func (d *Discovery) TailscaleHost() string {
	return d.tailscaleHost
}

// TailscaleIP returns the Tailscale IPv4 address, or empty if not available.
func (d *Discovery) TailscaleIP() string {
	return d.tailscaleIP
}

// SetTailscaleHost overrides the Tailscale hostname and IP, e.g. when using
// an embedded tsnet node whose identity is separate from the system Tailscale.
// Call before StartMDNS so the values are included in TXT records.
func (d *Discovery) SetTailscaleHost(host, ip string) {
	d.tailscaleHost = host
	d.tailscaleIP = ip
}

// Close stops mDNS advertisement.
func (d *Discovery) Close() {
	if d.dnssdCmd != nil && d.dnssdCmd.Process != nil {
		d.dnssdCmd.Process.Kill()
		d.dnssdCmd.Wait()
		log.Printf("[discovery] mDNS stopped")
	}
}
