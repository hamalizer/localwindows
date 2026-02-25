package discovery

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"localwindows/internal/protocol"
)

const (
	DiscoveryPort      = 19284
	broadcastInterval  = 2 * time.Second
	listenTimeout      = 5 * time.Second
	maxBroadcastPacket = 1024
)

// Host represents a discovered remote desktop host.
type Host struct {
	Name     string            `json:"name"`
	IP       string            `json:"ip"`
	Port     int               `json:"port"`
	AuthMode protocol.AuthMode `json:"auth_mode"`
	Version  uint8             `json:"version"`
	LastSeen time.Time         `json:"-"`
}

// Broadcaster announces this host's presence on the LAN via UDP broadcast.
type Broadcaster struct {
	host   Host
	conn   *net.UDPConn
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewBroadcaster creates a broadcaster that advertises the given host info.
func NewBroadcaster(host Host) *Broadcaster {
	return &Broadcaster{
		host:   host,
		stopCh: make(chan struct{}),
	}
}

// Start begins broadcasting on all network interfaces.
func (b *Broadcaster) Start() error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", DiscoveryPort))
	if err != nil {
		return fmt.Errorf("resolve broadcast addr: %w", err)
	}
	b.conn, err = net.ListenUDP("udp4", addr)
	if err != nil {
		// Port might be in use; try any port for sending.
		b.conn, err = net.ListenUDP("udp4", nil)
		if err != nil {
			return fmt.Errorf("listen udp: %w", err)
		}
	}

	b.wg.Add(1)
	go b.broadcastLoop()
	return nil
}

func (b *Broadcaster) broadcastLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()

	data, _ := json.Marshal(b.host)
	dst := &net.UDPAddr{IP: net.IPv4bcast, Port: DiscoveryPort}

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			// Broadcast on 255.255.255.255
			b.conn.WriteToUDP(data, dst)
			// Also broadcast on each interface's broadcast address.
			b.broadcastOnInterfaces(data)
		}
	}
}

func (b *Broadcaster) broadcastOnInterfaces(data []byte) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagBroadcast == 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			bcast := broadcastAddr(ipNet)
			if bcast != nil {
				dst := &net.UDPAddr{IP: bcast, Port: DiscoveryPort}
				b.conn.WriteToUDP(data, dst)
			}
		}
	}
}

func broadcastAddr(n *net.IPNet) net.IP {
	ip := n.IP.To4()
	if ip == nil {
		return nil
	}
	mask := n.Mask
	bcast := make(net.IP, 4)
	for i := range ip {
		bcast[i] = ip[i] | ^mask[i]
	}
	return bcast
}

// Stop halts broadcasting.
func (b *Broadcaster) Stop() {
	close(b.stopCh)
	if b.conn != nil {
		b.conn.Close()
	}
	b.wg.Wait()
}

// UpdateHost updates the advertised host info.
func (b *Broadcaster) UpdateHost(h Host) {
	b.host = h
}

// Listener discovers hosts on the LAN.
type Listener struct {
	mu     sync.RWMutex
	hosts  map[string]*Host // keyed by "ip:port"
	conn   *net.UDPConn
	stopCh chan struct{}
	wg     sync.WaitGroup
	onFound func(Host)
}

// NewListener creates a discovery listener.
func NewListener(onFound func(Host)) *Listener {
	return &Listener{
		hosts:   make(map[string]*Host),
		stopCh:  make(chan struct{}),
		onFound: onFound,
	}
}

// Start begins listening for host broadcasts.
func (l *Listener) Start() error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", DiscoveryPort))
	if err != nil {
		return err
	}
	l.conn, err = net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("listen discovery: %w", err)
	}
	l.wg.Add(1)
	go l.listenLoop()
	return nil
}

func (l *Listener) listenLoop() {
	defer l.wg.Done()
	buf := make([]byte, maxBroadcastPacket)

	for {
		select {
		case <-l.stopCh:
			return
		default:
		}

		l.conn.SetReadDeadline(time.Now().Add(listenTimeout))
		n, remote, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				l.pruneStale()
				continue
			}
			select {
			case <-l.stopCh:
				return
			default:
				log.Printf("discovery read error: %v", err)
				continue
			}
		}

		var host Host
		if err := json.Unmarshal(buf[:n], &host); err != nil {
			continue
		}
		if host.IP == "" {
			host.IP = remote.IP.String()
		}
		host.LastSeen = time.Now()

		key := fmt.Sprintf("%s:%d", host.IP, host.Port)
		l.mu.Lock()
		existing, found := l.hosts[key]
		if !found {
			l.hosts[key] = &host
			l.mu.Unlock()
			if l.onFound != nil {
				l.onFound(host)
			}
		} else {
			existing.LastSeen = host.LastSeen
			existing.Name = host.Name
			existing.AuthMode = host.AuthMode
			l.mu.Unlock()
		}
	}
}

func (l *Listener) pruneStale() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-10 * time.Second)
	for k, h := range l.hosts {
		if h.LastSeen.Before(cutoff) {
			delete(l.hosts, k)
		}
	}
}

// Hosts returns currently known hosts.
func (l *Listener) Hosts() []Host {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]Host, 0, len(l.hosts))
	for _, h := range l.hosts {
		result = append(result, *h)
	}
	return result
}

// Stop halts the listener.
func (l *Listener) Stop() {
	close(l.stopCh)
	if l.conn != nil {
		l.conn.Close()
	}
	l.wg.Wait()
}

// GetLocalIP returns the first non-loopback IPv4 address.
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}
