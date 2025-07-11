package mocknet

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestNotifications(t *testing.T) {
	const swarmSize = 5
	const timeout = 10 * time.Second

	mn, err := FullMeshLinked(swarmSize)
	if err != nil {
		t.Fatal(err)
	}
	defer mn.Close()

	// signup notifs
	nets := mn.Nets()
	notifiees := make(map[peer.ID]*netNotifiee, len(nets))
	for _, pn := range nets {
		defer pn.Close()

		n := newNetNotifiee(t, swarmSize)
		pn.Notify(n)
		notifiees[pn.LocalPeer()] = n
	}

	// connect all but self
	if err := mn.ConnectAllButSelf(); err != nil {
		t.Fatal(err)
	}

	// test everyone got the correct connection opened calls
	for _, s1 := range nets {
		n := notifiees[s1.LocalPeer()]
		notifs := make(map[peer.ID][]network.Conn)
		for _, s2 := range nets {
			if s2 == s1 {
				continue
			}

			// this feels a little sketchy, but its probably okay
			for len(s1.ConnsToPeer(s2.LocalPeer())) != len(notifs[s2.LocalPeer()]) {
				select {
				case c := <-n.connected:
					nfp := notifs[c.RemotePeer()]
					notifs[c.RemotePeer()] = append(nfp, c)
				case <-time.After(timeout):
					t.Fatal("timeout")
				}
			}
		}

		for p, cons := range notifs {
			expect := s1.ConnsToPeer(p)
			if len(expect) != len(cons) {
				t.Fatal("got different number of connections")
			}

			for _, c := range cons {
				var found bool
				for _, c2 := range expect {
					if c == c2 {
						found = true
						break
					}
				}

				if !found {
					t.Fatal("connection not found!")
				}
			}
		}
	}

	acceptedStream := make(chan struct{}, 1000)
	for _, s := range nets {
		s.SetStreamHandler(func(s network.Stream) {
			acceptedStream <- struct{}{}
			s.Close()
		})
	}

	// Make sure we've received at last one stream per conn.
	for _, s := range nets {
		conns := s.Conns()
		for _, c := range conns {
			st1, err := c.NewStream(context.Background())
			if err != nil {
				t.Error(err)
				continue
			}
			t.Logf("%s %s <--%p--> %s %s", c.LocalPeer(), c.LocalMultiaddr(), st1, c.RemotePeer(), c.RemoteMultiaddr())
			st1.Close()
		}
	}

	// close conns
	for _, s1 := range nets {
		n1 := notifiees[s1.LocalPeer()]
		for _, c1 := range s1.Conns() {
			c2 := ConnComplement(c1)

			n2 := notifiees[c2.LocalPeer()]
			c1.Close()

			var c3, c4 network.Conn
			select {
			case c3 = <-n1.disconnected:
			case <-time.After(timeout):
				t.Fatal("timeout")
			}
			if c1 != c3 {
				t.Fatal("got incorrect conn", c1, c3)
			}

			select {
			case c4 = <-n2.disconnected:
			case <-time.After(timeout):
				t.Fatal("timeout")
			}
			if c2 != c4 {
				t.Fatal("got incorrect conn", c1, c2)
			}
		}
	}

	for _, n1 := range notifiees {
		// Avoid holding this lock while waiting, otherwise we can deadlock.
		streamStateCopy := map[network.Stream]chan struct{}{}
		n1.streamState.Lock()
		for str, ch := range n1.streamState.m {
			streamStateCopy[str] = ch
		}
		n1.streamState.Unlock()

		for str1, ch1 := range streamStateCopy {
			<-ch1
			str2 := StreamComplement(str1)
			n2 := notifiees[str1.Conn().RemotePeer()]

			// make sure the OpenedStream notification was processed first
			var ch2 chan struct{}
			require.Eventually(t, func() bool {
				n2.streamState.Lock()
				defer n2.streamState.Unlock()
				ch, ok := n2.streamState.m[str2]
				if ok {
					ch2 = ch
				}
				return ok
			}, time.Second, 10*time.Millisecond)

			<-ch2
		}
	}
}

type netNotifiee struct {
	t *testing.T

	listen       chan ma.Multiaddr
	listenClose  chan ma.Multiaddr
	connected    chan network.Conn
	disconnected chan network.Conn

	streamState struct {
		sync.Mutex
		m map[network.Stream]chan struct{}
	}
}

func newNetNotifiee(t *testing.T, buffer int) *netNotifiee {
	nn := &netNotifiee{
		t:            t,
		listen:       make(chan ma.Multiaddr, 1),
		listenClose:  make(chan ma.Multiaddr, 1),
		connected:    make(chan network.Conn, buffer*2),
		disconnected: make(chan network.Conn, buffer*2),
	}
	nn.streamState.m = make(map[network.Stream]chan struct{})
	return nn
}

func (nn *netNotifiee) Listen(_ network.Network, a ma.Multiaddr) {
	nn.listen <- a
}
func (nn *netNotifiee) ListenClose(_ network.Network, a ma.Multiaddr) {
	nn.listenClose <- a
}
func (nn *netNotifiee) Connected(_ network.Network, v network.Conn) {
	nn.connected <- v
}
func (nn *netNotifiee) Disconnected(_ network.Network, v network.Conn) {
	nn.disconnected <- v
}
