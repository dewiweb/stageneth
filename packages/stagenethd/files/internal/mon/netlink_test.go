package mon

import (
	"encoding/binary"
	"net"
	"testing"
)

// buildMdbMsg fabricates one RTM_NEWMDB message: br_port_msg for bridge
// ifindex brIdx followed by nested MDBA_MDB → MDBA_MDB_ENTRY →
// MDBA_MDB_ENTRY_INFO(br_mdb_entry) attributes.
func buildMdbMsg(brIdx, portIdx uint32, vid uint16, group net.IP) []byte {
	entry := make([]byte, brMdbEntryLen)
	binary.NativeEndian.PutUint32(entry[0:4], portIdx)
	entry[4] = 0 // state
	entry[5] = 0 // flags
	binary.NativeEndian.PutUint16(entry[6:8], vid)
	if v4 := group.To4(); v4 != nil {
		copy(entry[8:12], v4)
		binary.BigEndian.PutUint16(entry[24:26], 0x0800)
	} else {
		copy(entry[8:24], group.To16())
		binary.BigEndian.PutUint16(entry[24:26], 0x86dd)
	}

	attr := func(typ uint16, payload []byte) []byte {
		l := 4 + len(payload)
		b := make([]byte, (l+rtAttrAlign-1)&^(rtAttrAlign-1))
		binary.NativeEndian.PutUint16(b[0:2], uint16(l))
		binary.NativeEndian.PutUint16(b[2:4], typ)
		copy(b[4:], payload)
		return b
	}
	inner := attr(mdbaMDBEntryInfo, entry)
	entryAttr := attr(mdbaMDBEntry, inner)
	mdbAttr := attr(mdbaMDB, entryAttr)

	msg := make([]byte, brPortMsgLen)
	msg[0] = afBridge
	binary.NativeEndian.PutUint32(msg[4:8], brIdx)
	return append(msg, mdbAttr...)
}

func TestParseMdbEntryIPv4(t *testing.T) {
	cache := map[uint32]string{}
	msg := buildMdbMsg(10, 20, 20, net.ParseIP("239.69.10.1"))
	entries := parseMdbAttrs(msg[brPortMsgLen:], cache)
	if len(entries) != 1 {
		t.Fatalf("entries = %v", entries)
	}
	e := entries[0]
	if e["group"] != "239.69.10.1" || e["vid"] != "20" {
		t.Errorf("entry = %v", e)
	}
	if e["port"] == "" {
		t.Error("port name missing")
	}
}

func TestParseMdbEntryIPv6(t *testing.T) {
	cache := map[uint32]string{}
	msg := buildMdbMsg(10, 20, 0, net.ParseIP("ff02::1"))
	entries := parseMdbAttrs(msg[brPortMsgLen:], cache)
	if len(entries) != 1 || entries[0]["group"] != "ff02::1" {
		t.Fatalf("entries = %v", entries)
	}
	if _, ok := entries[0]["vid"]; ok {
		t.Error("vid 0 should be omitted")
	}
}

func TestParseMdbEntryTruncated(t *testing.T) {
	if m := parseMdbEntry([]byte{1, 2, 3}, map[uint32]string{}); m != nil {
		t.Error("truncated entry should return nil")
	}
	bad := make([]byte, brMdbEntryLen)
	binary.BigEndian.PutUint16(bad[24:26], 0x1234) // unknown proto
	if m := parseMdbEntry(bad, map[uint32]string{}); m != nil {
		t.Error("unknown proto should return nil")
	}
}

func TestNetlinkAttrsTruncated(t *testing.T) {
	if got := netlinkAttrs([]byte{0xff, 0x00}); len(got) != 0 {
		t.Error("short buffer")
	}
	// declared length beyond buffer → stop
	b := make([]byte, 4)
	binary.NativeEndian.PutUint16(b[0:2], 100)
	if got := netlinkAttrs(b); len(got) != 0 {
		t.Error("overlong attr should stop")
	}
}

func TestBridgeMdbNetlinkLive(t *testing.T) {
	// on hosts without bridge MDB support the dump errors — either path
	// must return a well-formed map, never panic
	m, err := bridgeMdbNetlink()
	if err != nil {
		t.Skip("netlink MDB unavailable:", err)
	}
	for br, v := range m {
		if br == "" {
			t.Error("empty bridge name")
		}
		entries, ok := v.([]map[string]string)
		if !ok {
			t.Fatalf("bad entry type: %T", v)
		}
		for _, e := range entries {
			if e["group"] == "" {
				t.Error("entry missing group")
			}
		}
	}
}
