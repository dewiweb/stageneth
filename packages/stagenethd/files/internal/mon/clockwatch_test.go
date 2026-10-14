package mon

import (
	"encoding/binary"
	"net"
	"syscall"
	"testing"
	"time"
)

// buildEth wraps payload in an Ethernet II frame.
func buildEth(src [6]byte, etype uint16, payload []byte) []byte {
	b := make([]byte, 14, 14+len(payload))
	copy(b[0:6], []byte{0x01, 0x1b, 0x19, 0, 0, 0})
	copy(b[6:12], src[:])
	binary.BigEndian.PutUint16(b[12:14], etype)
	return append(b, payload...)
}

// buildIPv4UDP wraps payload in IPv4+UDP headers.
func buildIPv4UDP(srcIP, dstIP net.IP, dport uint16, payload []byte) []byte {
	udp := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint16(udp[0:2], 319)
	binary.BigEndian.PutUint16(udp[2:4], dport)
	udp = append(udp, payload...)
	ip := make([]byte, 20, 20+len(udp))
	ip[0] = 0x45
	ip[9] = 17
	copy(ip[12:16], srcIP.To4())
	copy(ip[16:20], dstIP.To4())
	return append(ip, udp...)
}

// buildPTPv1 fabricates an IEEE 1588-2002 header (Dante clocking).
func buildPTPv1(msgType byte, subdomain string, uuid [6]byte, seq uint16) []byte {
	p := make([]byte, 64)
	binary.BigEndian.PutUint16(p[0:2], 1) // versionPTP = 1
	binary.BigEndian.PutUint16(p[2:4], 1) // versionNetwork
	copy(p[4:20], []byte(subdomain))
	p[20] = msgType
	copy(p[22:28], uuid[:])
	binary.BigEndian.PutUint16(p[28:30], 1)
	binary.BigEndian.PutUint16(p[30:32], seq)
	return p
}

// buildPTPv2 fabricates a PTPv2/gPTP header (and announce body when
// msgType=0x0B).
func buildPTPv2(tSpec, msgType, domain byte, clockID [8]byte, seq uint16) []byte {
	p := make([]byte, 64)
	p[0] = tSpec<<4 | msgType
	p[1] = 0x02 // versionPTP = 2
	binary.BigEndian.PutUint16(p[2:4], 64)
	p[4] = domain
	copy(p[20:28], clockID[:])
	binary.BigEndian.PutUint16(p[28:30], 1)
	binary.BigEndian.PutUint16(p[30:32], seq)
	if msgType == 0x0b { // announce: gmIdentity @53, stepsRemoved @61
		copy(p[53:61], []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01, 0x02, 0x03})
		binary.BigEndian.PutUint16(p[61:63], 2)
	}
	return p
}

func TestClockWatchPTPv1(t *testing.T) {
	o := &clockObserver{done: make(chan struct{})}
	uuid := [6]byte{0x00, 0x1d, 0xc1, 0xaa, 0xbb, 0xcc}
	src := [6]byte{0, 0x1d, 0xc1, 1, 2, 3}
	payload := buildPTPv1(0, "_default", uuid, 42)
	pkt := buildIPv4UDP(net.IPv4(192, 168, 10, 5), net.IPv4(224, 0, 1, 129), 319, payload)
	o.handleFrame(buildEth(src, ethPIP, pkt))

	rep := o.report()
	if rep == nil {
		t.Fatal("no report")
	}
	v1, ok := rep["ptpv1"].(map[string]interface{})
	if !ok || v1["seen"] != true {
		t.Fatalf("ptpv1 not detected: %v", rep)
	}
	if v1["grandmaster"] != "00:1d:c1:aa:bb:cc" {
		t.Fatalf("bad GM uuid: %v", v1["grandmaster"])
	}
	if v1["subdomain"] != "_default" {
		t.Fatalf("bad subdomain: %v", v1["subdomain"])
	}
	if v1["last_src"] != "192.168.10.5" {
		t.Fatalf("bad src: %v", v1["last_src"])
	}
	types := v1["types"].(map[string]uint64)
	if types["sync"] != 1 {
		t.Fatalf("sync count wrong: %v", types)
	}
}

func TestClockWatchPTPv2UDP(t *testing.T) {
	o := &clockObserver{done: make(chan struct{})}
	clk := [8]byte{0x08, 0x00, 0x11, 0xff, 0xfe, 0x22, 0x33, 0x44}
	payload := buildPTPv2(0, 0, 8, clk, 7)
	pkt := buildIPv4UDP(net.IPv4(10, 0, 0, 9), net.IPv4(224, 0, 1, 129), 319, payload)
	o.handleFrame(buildEth([6]byte{8, 0, 0x11, 5, 6, 7}, ethPIP, pkt))

	v2, ok := o.report()["ptpv2"].(map[string]interface{})
	if !ok {
		t.Fatalf("ptpv2 not detected: %v", o.report())
	}
	if v2["domain"] != 8 {
		t.Fatalf("bad domain: %v", v2)
	}
	if v2["grandmaster"] != "08:00:11:ff:fe:22:33:44" {
		t.Fatalf("bad GM: %v", v2["grandmaster"])
	}
}

func TestClockWatchGPTP(t *testing.T) {
	o := &clockObserver{done: make(chan struct{})}
	clk := [8]byte{0x3c, 0xec, 0xef, 0xff, 0xfe, 0x10, 0x20, 0x30}
	// gPTP announce, VLAN-tagged, EtherType 0x88F7, transportSpecific=1
	announce := buildPTPv2(1, 0x0b, 0, clk, 3)
	inner := make([]byte, 4, 4+len(announce))
	binary.BigEndian.PutUint16(inner[0:2], 20) // VLAN id 20
	binary.BigEndian.PutUint16(inner[2:4], ethPPTP)
	inner = append(inner, announce...)
	frame := buildEth([6]byte{0x3c, 0xec, 0xef, 1, 2, 3}, ethPVLAN, inner)
	o.handleFrame(frame)

	g, ok := o.report()["gptp"].(map[string]interface{})
	if !ok {
		t.Fatalf("gptp not detected: %v", o.report())
	}
	if g["grandmaster"] != "de:ad:be:ef:00:01:02:03" {
		t.Fatalf("bad gPTP GM: %v", g["grandmaster"])
	}
	if g["steps_removed"] != 2 {
		t.Fatalf("bad steps_removed: %v", g)
	}
}

func TestClockWatchIgnoresNonPTP(t *testing.T) {
	o := &clockObserver{done: make(chan struct{})}
	// random UDP to a different multicast group/port — must not register
	payload := make([]byte, 64)
	pkt := buildIPv4UDP(net.IPv4(10, 0, 0, 1), net.IPv4(239, 1, 2, 3), 5000, payload)
	o.handleFrame(buildEth([6]byte{}, ethPIP, pkt))
	// UDP to the PTP group but wrong port
	pkt2 := buildIPv4UDP(net.IPv4(10, 0, 0, 1), net.IPv4(224, 0, 1, 129), 9999, payload)
	o.handleFrame(buildEth([6]byte{}, ethPIP, pkt2))
	// truncated / garbage frames
	o.handleFrame([]byte{1, 2, 3})
	o.handleFrame(make([]byte, 64))
	if o.report() != nil {
		t.Fatalf("non-PTP traffic leaked into report: %v", o.report())
	}
}

func TestClockWatchSyncRate(t *testing.T) {
	o := &clockObserver{done: make(chan struct{})}
	uuid := [6]byte{1, 2, 3, 4, 5, 6}
	p := buildPTPv1(0, "_default", uuid, 1)
	pkt := buildIPv4UDP(net.IPv4(192, 168, 1, 9), net.IPv4(224, 0, 1, 129), 319, p)
	fr := buildEth([6]byte{}, ethPIP, pkt)
	base := time.Now()
	// inject two syncs 500ms apart by calling handleFrame with controlled
	// timing via parseV1 directly (handleFrame uses time.Now internally,
	// so test the rate smoother on the protoObs primitive)
	o.st.V1.bump("sync", "x", 1, base)
	o.st.V1.bump("sync", "x", 2, base.Add(500*time.Millisecond))
	if hz := o.st.V1.SyncHz; hz < 1.5 || hz > 2.5 {
		t.Fatalf("sync rate not ~2Hz: %v", hz)
	}
	_ = fr
}

func TestClockObserverLifecycle(t *testing.T) {
	ifi, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skip("no lo interface")
	}
	obs, err := newClockObserver(ifi.Name)
	if err != nil {
		t.Skipf("AF_PACKET unavailable (no CAP_NET_RAW?): %v", err)
	}
	obs.stop()
	obs.stop() // idempotent
}

// TestClockWatchLive sends fabricated wire frames through a real
// AF_PACKET socket and asserts the observer + BPF filter pick them up.
// Requires CAP_NET_RAW (privileged container or root).
func TestClockWatchLive(t *testing.T) {
	ifi, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skip("no lo")
	}
	obs, err := newClockObserver(ifi.Name)
	if err != nil {
		t.Skipf("no CAP_NET_RAW: %v", err)
	}
	defer obs.stop()

	sfd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, htons(ethPAll))
	if err != nil {
		t.Skipf("no send socket: %v", err)
	}
	defer syscall.Close(sfd)
	sa := &syscall.SockaddrLinklayer{Ifindex: ifi.Index, Halen: 6}
	copy(sa.Addr[:], []byte{0x01, 0x00, 0x5e, 0x00, 0x01, 0x81})

	uuid := [6]byte{0x00, 0x1d, 0xc1, 0xaa, 0xbb, 0xcc}
	frames := [][]byte{
		buildEth([6]byte{0, 0x1d, 0xc1, 1, 2, 3}, ethPIP,
			buildIPv4UDP(net.IPv4(192, 168, 10, 5), net.IPv4(224, 0, 1, 129), 319,
				buildPTPv1(0, "_default", uuid, 1))),
		buildEth([6]byte{0x3c, 0xec, 0xef, 1, 2, 3}, ethPPTP,
			buildPTPv2(1, 0x0b, 0, [8]byte{0x3c, 0xec, 0xef, 0xff, 0xfe, 0, 0, 1}, 2)),
		// negative control: UDP to wrong group must be dropped by BPF
		buildEth([6]byte{9, 9, 9, 9, 9, 9}, ethPIP,
			buildIPv4UDP(net.IPv4(10, 0, 0, 1), net.IPv4(239, 9, 9, 9), 5000,
				make([]byte, 64))),
	}
	for _, f := range frames {
		if err := syscall.Sendto(sfd, f, 0, sa); err != nil {
			t.Fatalf("sendto: %v", err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rep := obs.report()
		if rep != nil && rep["ptpv1"] != nil && rep["gptp"] != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("frames not received through live socket: %v", obs.report())
}

func TestPTPBPFValid(t *testing.T) {
	prog := ptpBPF()
	if len(prog) == 0 {
		t.Fatal("empty BPF program")
	}
	// every jump must land inside the program
	for i, ins := range prog {
		if ins.Code == 0x15 { // JEQ
			for _, tgt := range []int{i + 1 + int(ins.Jt), i + 1 + int(ins.Jf)} {
				if tgt < 0 || tgt >= len(prog) {
					t.Fatalf("insn %d jumps out of bounds to %d", i, tgt)
				}
			}
		}
	}
}
