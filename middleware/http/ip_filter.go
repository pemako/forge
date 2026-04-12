package httpMiddleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

// IPFilterConfig configures the IP allowlist/blocklist middleware.
type IPFilterConfig struct {
	// Mode is either "whitelist" (allow only listed IPs) or "blacklist" (block listed IPs).
	Mode string
	// IPs is a list of individual IP addresses or CIDR ranges (e.g., "192.168.1.1", "10.0.0.0/8").
	IPs []string
	// OnDeny is called when a request is denied. Defaults to returning HTTP 403 Forbidden.
	OnDeny func(*gin.Context)
}

// IPFilter returns a Gin middleware that allows or blocks requests based on the
// client's IP address. In "whitelist" mode, only listed IPs are allowed through;
// all others receive the OnDeny response. In "blacklist" mode, listed IPs are
// blocked and all others are allowed through.
func IPFilter(cfg IPFilterConfig) gin.HandlerFunc {
	nets := parseCIDRs(cfg.IPs)
	onDeny := cfg.OnDeny
	if onDeny == nil {
		onDeny = func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusForbidden, errors.ErrForbidden)
		}
	}
	return func(c *gin.Context) {
		ip := net.ParseIP(c.ClientIP())
		matched := matchIP(ip, nets)
		if (cfg.Mode == "whitelist" && !matched) || (cfg.Mode == "blacklist" && matched) {
			onDeny(c)
			return
		}
		c.Next()
	}
}

func parseCIDRs(ips []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, s := range ips {
		if strings.Contains(s, "/") {
			_, n, err := net.ParseCIDR(s)
			if err == nil {
				nets = append(nets, n)
			}
		} else {
			ip := net.ParseIP(s)
			if ip != nil {
				mask := net.CIDRMask(32, 32)
				if ip.To4() == nil {
					mask = net.CIDRMask(128, 128)
				}
				nets = append(nets, &net.IPNet{IP: ip, Mask: mask})
			}
		}
	}
	return nets
}

func matchIP(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
