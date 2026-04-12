package grpcMiddleware

import (
	"context"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// GRPCIPFilterConfig configures the IP allowlist/blocklist interceptor.
type GRPCIPFilterConfig struct {
	// Mode is either "whitelist" (allow only listed IPs) or "blacklist" (block listed IPs).
	Mode string
	// IPs is a list of individual IP addresses or CIDR ranges (e.g., "10.0.0.1", "192.168.0.0/24").
	IPs []string
}

// UnaryIPFilter returns a gRPC unary server interceptor that allows or blocks
// requests based on the caller's IP address derived from the peer info in the
// context. In "whitelist" mode, only listed IPs are allowed; all others receive
// codes.PermissionDenied. In "blacklist" mode, listed IPs are denied.
func UnaryIPFilter(cfg GRPCIPFilterConfig) grpc.UnaryServerInterceptor {
	nets := parseGRPCCIDRs(cfg.IPs)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		addr := ""
		if p, ok := peer.FromContext(ctx); ok {
			addr = p.Addr.String()
		}
		host, _, _ := net.SplitHostPort(addr)
		ip := net.ParseIP(host)
		matched := matchGRPCIP(ip, nets)
		if (cfg.Mode == "whitelist" && !matched) || (cfg.Mode == "blacklist" && matched) {
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}
		return handler(ctx, req)
	}
}

func parseGRPCCIDRs(ips []string) []*net.IPNet {
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

func matchGRPCIP(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
