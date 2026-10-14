package mon

// Bridge multicast DB via rtnetlink — the native equivalent of
// `bridge mdb show`, with zero forks. Falls back to the bounded exec
// if the netlink dump fails (e.g. bridged module absent).

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"
)

// rtnetlink/if_bridge.h constants not exported by x/sys/unix.
const (
	rtmNewMDB        = 84 // RTM_NEWMDB
	rtmGetMDB        = 86 // RTM_GETMDB
	afBridge         = 7  // AF_BRIDGE
	mdbaMDB          = 1  // MDBA_MDB
	mdbaMDBEntry     = 1  // MDBA_MDB_ENTRY
	mdbaMDBEntryInfo = 1  // MDBA_MDB_ENTRY_INFO
	rtAttrAlign      = 4
)

// br_port_msg: family(1) + pad(3) + ifindex(4)
const brPortMsgLen = 8

// br_mdb_entry: ifindex(4) + state(1) + flags(1) + vid(2) + addr.u(16) + proto(2)
const brMdbEntryLen = 26

// parseBridgeMdb dumps the MDB via netlink; falls back to `bridge mdb show`.
func parseBridgeMdb() map[string]interface{} {
	if m, err := bridgeMdbNetlink(); err == nil {
		return m
	}
	b, err := execBounded(3*time.Second, "bridge", "mdb", "show")
	if err != nil {
		return map[string]interface{}{}
	}
	return parseMdbOutput(string(b))
}

func bridgeMdbNetlink() (map[string]interface{}, error) {
	out := map[string]interface{}{}
	tab, err := syscall.NetlinkRIB(rtmGetMDB, afBridge)
	if err != nil {
		return nil, err
	}
	msgs, err := syscall.ParseNetlinkMessage(tab)
	if err != nil {
		return nil, err
	}
	ifByIndex := map[uint32]string{}
	for _, msg := range msgs {
		if msg.Header.Type != rtmNewMDB || len(msg.Data) < brPortMsgLen {
			continue
		}
		brIndex := binary.NativeEndian.Uint32(msg.Data[4:8])
		entries := parseMdbAttrs(msg.Data[brPortMsgLen:], ifByIndex)
		if len(entries) == 0 {
			continue
		}
		br := ifaceName(ifByIndex, brIndex)
		list, _ := out[br].([]map[string]string)
		out[br] = append(list, entries...)
	}
	return out, nil
}

// ifaceName resolves an ifindex to its interface name, cached.
func ifaceName(cache map[uint32]string, idx uint32) string {
	if n, ok := cache[idx]; ok {
		return n
	}
	name := fmt.Sprintf("if%d", idx)
	if ifi, err := net.InterfaceByIndex(int(idx)); err == nil {
		name = ifi.Name
	}
	cache[idx] = name
	return name
}

// parseMdbAttrs walks the nested attribute tree:
// MDBA_MDB → [MDBA_MDB_ENTRY]* → MDBA_MDB_ENTRY_INFO → br_mdb_entry
func parseMdbAttrs(data []byte, ifByIndex map[uint32]string) []map[string]string {
	var out []map[string]string
	for _, a := range netlinkAttrs(data) {
		if a.typ != mdbaMDB {
			continue
		}
		for _, e := range netlinkAttrs(a.payload) {
			if e.typ != mdbaMDBEntry {
				continue
			}
			for _, info := range netlinkAttrs(e.payload) {
				if info.typ != mdbaMDBEntryInfo {
					continue
				}
				if m := parseMdbEntry(info.payload, ifByIndex); m != nil {
					out = append(out, m)
				}
			}
		}
	}
	return out
}

// parseMdbEntry decodes one br_mdb_entry struct.
func parseMdbEntry(b []byte, ifByIndex map[uint32]string) map[string]string {
	if len(b) < brMdbEntryLen {
		return nil
	}
	portIdx := binary.NativeEndian.Uint32(b[0:4])
	vid := binary.NativeEndian.Uint16(b[6:8])
	proto := binary.BigEndian.Uint16(b[24:26])
	var group string
	switch proto {
	case 0x0800: // IPv4
		group = net.IP(b[8:12]).String()
	case 0x86dd: // IPv6
		group = net.IP(b[8:24]).String()
	default:
		return nil
	}
	m := map[string]string{
		"port":  ifaceName(ifByIndex, portIdx),
		"group": group,
	}
	if vid != 0 {
		m["vid"] = fmt.Sprintf("%d", vid)
	}
	return m
}

type rtAttr struct {
	typ     uint16
	payload []byte
}

// netlinkAttrs parses a sequence of struct rtattr (len+type+payload,
// each padded to rtAttrAlign).
func netlinkAttrs(b []byte) []rtAttr {
	var out []rtAttr
	for len(b) >= 4 {
		l := int(binary.NativeEndian.Uint16(b[0:2]))
		if l < 4 || l > len(b) {
			break
		}
		out = append(out, rtAttr{typ: binary.NativeEndian.Uint16(b[2:4]), payload: b[4:l]})
		adv := (l + rtAttrAlign - 1) &^ (rtAttrAlign - 1)
		if adv > len(b) {
			break
		}
		b = b[adv:]
	}
	return out
}
