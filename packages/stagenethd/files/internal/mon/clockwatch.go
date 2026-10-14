// Passive PTP domain observer.
//
// The appliance never transmits clock traffic — it only listens — so it
// can never accidentally win a best-master election on a show network.
// One AF_PACKET socket per service bridge (br_*) watches for:
//
//   - PTPv1 (IEEE 1588-2002, Dante clocking): UDP multicast
//     224.0.1.129 ports 319/320, versionPTP=1 header
//   - PTPv2 over UDP/IPv4 (AES67 / RAVENNA / ST 2110-30): same groups
//     and ports, versionPTP=2
//   - gPTP (IEEE 802.1AS, AVB / MILAN): EtherType 0x88F7 with
//     transportSpecific (majorSdoId) = 1
//   - PTPv2 over L2: EtherType 0x88F7, transportSpecific = 0
//
// A compiled cBPF filter attached to the socket keeps high-rate AV
// media multicast out of userspace entirely: only 0x88F7 frames and
// UDP/319-320 datagrams are delivered.
package mon

import (
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	ethPAll      = 0x0003
	ethPPTP      = 0x88F7
	ethPIP       = 0x0800
	ethPVLAN     = 0x8100
	ethPQINQ     = 0x88A8
	solPacket    = 263
	soAttachFilt = 26
	pktAddMember = 1
	pktMrMulti   = 2
)

// ptpMcastAddrs are the L2 multicast destinations carrying clock
// traffic. Joining them on the observed interface counters IGMP
// snooping that would otherwise keep clock multicast off the local port.
var ptpMcastAddrs = [][6]byte{
	{0x01, 0x00, 0x5e, 0x00, 0x01, 0x81}, // 224.0.1.129 (PTPv1 all, PTPv2 primary)
	{0x01, 0x00, 0x5e, 0x00, 0x00, 0x6b}, // 224.0.0.107 (PTPv2 peer delay)
	{0x01, 0x1b, 0x19, 0x00, 0x00, 0x00}, // PTPv2/gPTP event messages (L2)
	{0x01, 0x80, 0xc2, 0x00, 0x00, 0x0e}, // PTPv2/gPTP peer-delay (L2)
}

// ptpBPF accepts EtherType 0x88F7 or IPv4/UDP dst port 319-320, with one
// level of VLAN tag unwrapping. Keeps high-rate AV media multicast out
// of userspace on service bridges.
func ptpBPF() []syscall.SockFilter {
	const (
		ldhAbs = 0x28 // BPF_LD|BPF_H|BPF_ABS
		ldbAbs = 0x30 // BPF_LD|BPF_B|BPF_ABS
		ldhInd = 0x48 // BPF_LD|BPF_H|BPF_IND
		ldxMsh = 0xb1 // BPF_LDX|BPF_MSH|BPF_B
		jeq    = 0x15 // BPF_JMP|BPF_JEQ|BPF_K
		retK   = 0x06 // BPF_RET|BPF_K
	)
	f := func(code uint16, jt, jf uint8, k uint32) syscall.SockFilter {
		return syscall.SockFilter{Code: code, Jt: jt, Jf: jf, K: k}
	}
	const (
		drop   = 20
		accept = 21
	)
	return []syscall.SockFilter{
		/* 0  */ f(ldhAbs, 0, 0, 12), // A = EtherType
		/* 1  */ f(jeq, accept-2, 0, ethPPTP),
		/* 2  */ f(jeq, 2, 0, ethPVLAN),
		/* 3  */ f(jeq, 1, 0, ethPQINQ),
		/* 4  */ f(jeq, 3, drop-5, ethPIP),
		/* 5  */ f(ldhAbs, 0, 0, 16), // inner EtherType after one tag
		/* 6  */ f(jeq, accept-7, 0, ethPPTP),
		/* 7  */ f(jeq, 6, drop-8, ethPIP),
		/* 8  */ f(ldbAbs, 0, 0, 23), // IP proto (IP header at 14)
		/* 9  */ f(jeq, 0, drop-10, 17), // UDP only
		/* 10 */ f(ldxMsh, 0, 0, 14), // X = IP header length
		/* 11 */ f(ldhInd, 0, 0, 16), // UDP dst port at X+14+2
		/* 12 */ f(jeq, accept-13, 0, 319),
		/* 13 */ f(jeq, accept-14, drop-14, 320),
		/* 14 */ f(ldbAbs, 0, 0, 27), // IP proto (IP header at 18)
		/* 15 */ f(jeq, 0, drop-16, 17),
		/* 16 */ f(ldxMsh, 0, 0, 18),
		/* 17 */ f(ldhInd, 0, 0, 20), // UDP dst port at X+18+2
		/* 18 */ f(jeq, accept-19, 0, 319),
		/* 19 */ f(jeq, accept-20, drop-20, 320),
		/* 20 */ f(retK, 0, 0, 0), // drop
		/* 21 */ f(retK, 0, 0, 0xffff), // accept
	}
}

// htons converts a 16-bit value to network byte order for AF_PACKET use.
func htons(v int) int { return (v&0xff)<<8 | (v>>8)&0xff }

// packetMreq mirrors struct packet_mreq — stdlib syscall has no
// SetsockoptPacketMreq wrapper, so we issue setsockopt directly.
type packetMreq struct {
	Ifindex int32
	Type    uint16
	Alen    uint16
	Address [8]byte
}

func setsockopt(fd, level, opt int, ptr unsafe.Pointer, sz uintptr) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_SETSOCKOPT,
		uintptr(fd), uintptr(level), uintptr(opt), uintptr(ptr), sz, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// --- per-protocol observation state -----------------------------------------

type protoObs struct {
	Seen      bool
	FirstSeen time.Time
	LastSeen  time.Time
	Count     uint64
	LastSrc   string // MAC or IP of last sender
	GM        string // grandmaster / sourceUuid identity
	Domain    int    // PTPv2 domainNumber (-1 = unset)
	Subdomain string // PTPv1 subdomain name
	Steps     int    // 802.1AS stepsRemoved (-1 = unset)
	LastSeq   uint16
	SyncHz    float64 // smoothed sync/follow-up rate
	lastSync  time.Time
	Types     map[string]uint64
}

func (p *protoObs) bump(msgType, src string, seq uint16, now time.Time) {
	if !p.Seen {
		p.FirstSeen = now
		p.Domain = -1
		p.Steps = -1
		p.Types = map[string]uint64{}
	}
	p.Seen = true
	p.LastSeen = now
	p.Count++
	p.LastSrc = src
	p.LastSeq = seq
	p.Types[msgType]++
	if msgType == "sync" || msgType == "follow_up" {
		if !p.lastSync.IsZero() {
			if dt := now.Sub(p.lastSync).Seconds(); dt > 0.1 && dt < 60 {
				rate := 1.0 / dt
				if p.SyncHz == 0 {
					p.SyncHz = rate
				} else {
					p.SyncHz = 0.75*p.SyncHz + 0.25*rate
				}
			}
		}
		p.lastSync = now
	}
}

func (p *protoObs) report() map[string]interface{} {
	if !p.Seen {
		return nil
	}
	m := map[string]interface{}{
		"seen":      true,
		"count":     p.Count,
		"last_seen": p.LastSeen.Unix(),
		"age_sec":   time.Since(p.LastSeen).Seconds(),
		"types":     p.Types,
	}
	if p.LastSrc != "" {
		m["last_src"] = p.LastSrc
	}
	if p.GM != "" {
		m["grandmaster"] = p.GM
	}
	if p.Domain >= 0 {
		m["domain"] = p.Domain
	}
	if p.Subdomain != "" {
		m["subdomain"] = p.Subdomain
	}
	if p.Steps >= 0 {
		m["steps_removed"] = p.Steps
	}
	if p.SyncHz > 0 {
		m["sync_hz"] = p.SyncHz
	}
	return m
}

// --- observer ----------------------------------------------------------------

type domainStats struct {
	V1   protoObs // Dante: IEEE 1588-2002 over UDP 224.0.1.129
	V2   protoObs // AES67/RAVENNA/ST2110-30: PTPv2 over UDP
	V2L2 protoObs // PTPv2 over EtherType 0x88F7 (transportSpecific 0)
	GPTP protoObs // 802.1AS over EtherType 0x88F7 (transportSpecific 1)
}

// clockObserver passively watches one interface for PTP-family traffic.
type clockObserver struct {
	iface string
	fd    int
	done  chan struct{}
	wg    sync.WaitGroup
	mu    sync.Mutex
	st    domainStats
}

var v1MsgTypes = map[byte]string{
	0: "sync", 1: "delay_req", 2: "follow_up", 3: "delay_resp", 4: "management",
}

var v2MsgTypes = map[byte]string{
	0: "sync", 1: "delay_req", 2: "pdelay_req", 3: "pdelay_resp",
	8: "follow_up", 9: "delay_resp", 10: "pdelay_resp_follow_up",
	11: "announce", 12: "signaling", 13: "management",
}

func newClockObserver(iface string) (*clockObserver, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, htons(ethPAll))
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrLinklayer{Protocol: uint16(htons(ethPAll)), Ifindex: ifi.Index}); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	for _, mac := range ptpMcastAddrs {
		mreq := packetMreq{Ifindex: int32(ifi.Index), Type: pktMrMulti, Alen: 6}
		copy(mreq.Address[:], mac[:])
		setsockopt(fd, solPacket, pktAddMember, unsafe.Pointer(&mreq), unsafe.Sizeof(mreq)) // best-effort
	}
	prog := ptpBPF()
	fprog := syscall.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	setsockopt(fd, solPacket, soAttachFilt, unsafe.Pointer(&fprog), unsafe.Sizeof(fprog)) // best-effort
	tv := syscall.NsecToTimeval(500 * int64(time.Millisecond))
	syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

	o := &clockObserver{iface: iface, fd: fd, done: make(chan struct{})}
	o.wg.Add(1)
	go o.loop()
	return o, nil
}

func (o *clockObserver) stop() {
	select {
	case <-o.done:
	default:
		close(o.done)
	}
	o.wg.Wait()
	syscall.Close(o.fd)
}

func (o *clockObserver) closed() bool {
	select {
	case <-o.done:
		return true
	default:
		return false
	}
}

func (o *clockObserver) loop() {
	defer o.wg.Done()
	buf := make([]byte, 2048)
	for !o.closed() {
		n, _, err := syscall.Recvfrom(o.fd, buf, 0)
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK || err == syscall.EINTR {
				continue
			}
			if o.closed() {
				return
			}
			continue
		}
		o.handleFrame(buf[:n])
	}
}

// handleFrame is pure — unit tests feed fabricated wire bytes.
func (o *clockObserver) handleFrame(b []byte) {
	if len(b) < 14 {
		return
	}
	src := hexBytes(b[6:12])
	et := int(binary.BigEndian.Uint16(b[12:14]))
	off := 14
	if (et == ethPVLAN || et == ethPQINQ) && len(b) >= off+4 {
		et = int(binary.BigEndian.Uint16(b[off+2 : off+4]))
		off += 4
	}
	now := time.Now()
	switch et {
	case ethPPTP:
		o.handlePTPL2(b[off:], src, now)
	case ethPIP:
		o.handleUDP(b[off:], now)
	}
}

// handleUDP pulls a PTP payload out of an IPv4/UDP datagram.
func (o *clockObserver) handleUDP(ip []byte, now time.Time) {
	if len(ip) < 20 || ip[0]>>4 != 4 || ip[9] != 17 {
		return
	}
	ihl := int(ip[0]&0x0f) * 4
	if len(ip) < ihl+8 {
		return
	}
	dst := net.IP(ip[16:20]).String()
	if dst != "224.0.1.129" && dst != "224.0.0.107" {
		return
	}
	dport := binary.BigEndian.Uint16(ip[ihl+2 : ihl+4])
	if dport != 319 && dport != 320 {
		return
	}
	src := net.IP(ip[12:16]).String()
	payload := ip[ihl+8:]
	if len(payload) < 34 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	// v1 (uint16@0 == 1) and v2 (low nibble of byte 1 == 2) can't collide:
	// a v2 message never has versionPTP field layout matching uint16 == 1.
	if binary.BigEndian.Uint16(payload[0:2]) == 1 {
		o.parseV1(payload, src, now)
		return
	}
	if payload[1]&0x0f != 2 {
		return
	}
	o.parseV2(payload, src, &o.st.V2, now)
}

// handlePTPL2 parses PTP carried directly on EtherType 0x88F7.
// transportSpecific (majorSdoId) = 1 marks IEEE 802.1AS (gPTP);
// 0 is generic PTPv2-over-L2.
func (o *clockObserver) handlePTPL2(p []byte, srcMAC string, now time.Time) {
	if len(p) < 34 || p[1]&0x0f != 2 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if p[0]>>4 == 1 {
		o.parseV2(p, srcMAC, &o.st.GPTP, now)
	} else {
		o.parseV2(p, srcMAC, &o.st.V2L2, now)
	}
}

// parseV2 updates a protocol bucket from a PTPv2 wire-format message
// (shared by UDP v2, L2 v2 and gPTP — same header layout).
func (o *clockObserver) parseV2(p []byte, src string, obs *protoObs, now time.Time) {
	msgType := p[0] & 0x0f
	obs.bump(v2MsgTypeName(msgType), src, binary.BigEndian.Uint16(p[30:32]), now)
	obs.Domain = int(p[4])
	switch msgType {
	case 0, 1, 2, 3, 8: // time-critical messages: sender is the clock itself
		if obs.GM == "" {
			obs.GM = hexBytes(p[20:28])
		}
	case 11: // announce carries the grandmaster identity explicitly
		if len(p) >= 64 {
			obs.GM = hexBytes(p[53:61])
			obs.Steps = int(binary.BigEndian.Uint16(p[61:63]))
		}
	}
}

func v2MsgTypeName(t byte) string {
	if s, ok := v2MsgTypes[t]; ok {
		return s
	}
	return "other"
}

func v1MsgTypeName(t byte) string {
	if s, ok := v1MsgTypes[t]; ok {
		return s
	}
	return "other"
}

// parseV1 decodes an IEEE 1588-2002 (PTPv1, Dante clocking) header:
// versionPTP(2)@0, versionNetwork(2)@2, subdomain(16)@4,
// messageType@20, sourceUuid(6)@22, sourcePortId(2)@28,
// sequenceId(2)@30, control@32.
func (o *clockObserver) parseV1(p []byte, srcIP string, now time.Time) {
	if len(p) < 34 {
		return
	}
	obs := &o.st.V1
	obs.bump(v1MsgTypeName(p[20]), srcIP, binary.BigEndian.Uint16(p[30:32]), now)
	obs.Subdomain = strings.TrimRight(string(p[4:20]), "\x00 ")
	if p[20] == 0 || p[20] == 2 { // sync / follow_up carry the GM uuid
		obs.GM = hexBytes(p[22:28])
	}
}

func hexBytes(b []byte) string {
	const hex = "0123456789abcdef"
	var sb strings.Builder
	for i, c := range b {
		if i > 0 {
			sb.WriteByte(':')
		}
		sb.WriteByte(hex[c>>4])
		sb.WriteByte(hex[c&0xf])
	}
	return sb.String()
}

func (o *clockObserver) report() map[string]interface{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := map[string]interface{}{}
	if m := o.st.V1.report(); m != nil {
		out["ptpv1"] = m
	}
	if m := o.st.V2.report(); m != nil {
		out["ptpv2"] = m
	}
	if m := o.st.V2L2.report(); m != nil {
		out["ptpv2_l2"] = m
	}
	if m := o.st.GPTP.report(); m != nil {
		out["gptp"] = m
	}
	if len(out) == 0 {
		return nil
	}
	out["interface"] = o.iface
	return out
}

// --- collector integration ---------------------------------------------------

// syncClockObservers keeps one observer per service bridge (br_*) —
// that is where Dante (PTPv1), AVB/MILAN (gPTP) and AES67 (PTPv2)
// traffic is actually visible. Observers for vanished interfaces are
// stopped so a deleted bridge can't leak a goroutine or fd.
func (c *Collector) syncClockObservers(ifaces []map[string]interface{}) {
	want := map[string]bool{}
	for _, iface := range ifaces {
		if name, _ := iface["name"].(string); strings.HasPrefix(name, "br_") {
			want[name] = true
		}
	}
	if c.clockObs == nil {
		c.clockObs = map[string]*clockObserver{}
	}
	for name, obs := range c.clockObs {
		if !want[name] {
			obs.stop()
			delete(c.clockObs, name)
		}
	}
	for name := range want {
		if _, ok := c.clockObs[name]; !ok {
			if obs, err := newClockObserver(name); err == nil {
				c.clockObs[name] = obs
			}
		}
	}
}

func (c *Collector) clockSnapshot() map[string]interface{} {
	out := map[string]interface{}{}
	for name, obs := range c.clockObs {
		if m := obs.report(); m != nil {
			out[name] = m
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Collector) stopClockObservers() {
	c.collectMu.Lock()
	defer c.collectMu.Unlock()
	for name, obs := range c.clockObs {
		obs.stop()
		delete(c.clockObs, name)
	}
}
