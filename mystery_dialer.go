package pxndscvm

import (
	"context"
	"fmt"
	"net"
	"time"

	"h12.io/socks"
)

// MysteryDialer is a dialer function that will use a different proxy for every request.
func (s *Swamp) MysteryDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	var sock Proxy
	sock = Proxy{Endpoint: ""}
	// pull down proxies from channel until we get a proxy good enough for our spoiled asses
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		time.Sleep(10 * time.Millisecond)
		candidate := s.GetAnySOCKS()
		if !s.stillGood(candidate) {
			s.dbgPrint("MysteryDialer cycle proxies, old: " + candidate.Endpoint)
			continue
		}

		sock = candidate

		if sock.Endpoint != "" {
			break
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	socksString := fmt.Sprintf("socks%s://%s?timeout=15s", sock.Proto, sock.Endpoint)
	s.dbgPrint("MysteryDialer using socks: " + socksString)
	var dialSocks = socks.Dial(socksString)
	return dialSocks(network, addr)
}