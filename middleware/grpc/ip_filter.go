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

type GRPCIPFilterConfig struct {
	Mode string
	IPs  []string
}

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
