package client

import (
	"context"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/transport"

	ma "github.com/multiformats/go-multiaddr"
)

var circuitProtocol = ma.ProtocolWithCode(ma.P_CIRCUIT)
var circuitAddr = ma.Cast(circuitProtocol.VCode)

// AddTransport constructs a new p2p-circuit/v2 client and adds it as a transport to the
// host network
func AddTransport(h host.Host, upgrader transport.Upgrader) error {
	n, ok := h.Network().(transport.TransportNetwork)
	if !ok {
		return fmt.Errorf("%v is not a transport network", h.Network())
	}

	c, err := New(h, upgrader)
	if err != nil {
		return fmt.Errorf("error constructing circuit client: %w", err)
	}

	err = n.AddTransport(c)
	if err != nil {
		return fmt.Errorf("error adding circuit transport: %w", err)
	}

	err = n.Listen(circuitAddr)
	if err != nil {
		return fmt.Errorf("error listening to circuit addr: %w", err)
	}

	c.Start()

	return nil
}

// Transport interface
var _ transport.Transport = (*Client)(nil)

// p2p-circuit implements the SkipResolver interface so that the underlying
// transport can do the address resolution later. If you wrap this transport,
// make sure you also implement SkipResolver as well.
var _ transport.SkipResolver = (*Client)(nil)
var _ io.Closer = (*Client)(nil)

// SkipResolve returns true since we always defer to the inner transport for
// the actual connection. By skipping resolution here, we let the inner
// transport decide how to resolve the multiaddr
func (c *Client) SkipResolve(_ context.Context, _ ma.Multiaddr) bool {
	return true
}

func (c *Client) Dial(ctx context.Context, a ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	connScope, err := c.host.Network().ResourceManager().OpenConnection(network.DirOutbound, false, a)

	if err != nil {
		return nil, err
	}
	conn, err := c.dialAndUpgrade(ctx, a, p, connScope)
	if err != nil {
		connScope.Done()
		return nil, err
	}
	return conn, nil
}

func (c *Client) dialAndUpgrade(ctx context.Context, a ma.Multiaddr, p peer.ID, connScope network.ConnManagementScope) (transport.CapableConn, error) {
	if err := connScope.SetPeer(p); err != nil {
		return nil, err
	}
	conn, err := c.dial(ctx, a, p)
	if err != nil {
		return nil, err
	}
	conn.tagHop()
	cc, err := c.upgrader.Upgrade(ctx, c, conn, network.DirOutbound, p, connScope)
	if err != nil {
		return nil, err
	}
	return capableConn{cc.(capableConnWithStat)}, nil
}

func (c *Client) CanDial(addr ma.Multiaddr) bool {
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

func (c *Client) Listen(addr ma.Multiaddr) (transport.Listener, error) {
	// TODO connect to the relay and reserve slot if specified
	if _, err := addr.ValueForProtocol(ma.P_CIRCUIT); err != nil {
		return nil, err
	}

	return c.upgrader.UpgradeGatedMaListener(c, c.upgrader.GateMaListener(c.Listener())), nil
}

func (c *Client) Protocols() []int {
	return []int{ma.P_CIRCUIT}
}

func (c *Client) Proxy() bool {
	return true
}
