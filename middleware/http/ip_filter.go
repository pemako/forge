package httpMiddleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

type IPFilterConfig struct {
	Mode   string
	IPs    []string
	OnDeny func(*gin.Context)
}

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
