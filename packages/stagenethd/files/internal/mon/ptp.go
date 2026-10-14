package mon

// Native linuxptp management client: speaks PTPv2 management messages
// over the unixgram socket ptp4l exposes at /var/run/pmc.<ifindex>,
// replacing the `pmc` subprocess. Falls back to the exec if the socket
// is absent or a reply doesn't parse.

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	ptpMsgManagement = 0x0d

	ptpActionGet      = 0
	ptpActionResponse = 2

	tlvManagement = 0x0001

	midPortDataSet    = 0x2004
	midCurrentDataSet = 0x2001
)

var ptpSeq uint16

var portStateNames = map[byte]string{
	1: "INITIALIZING", 2: "FAULTY", 3: "DISABLED", 4: "LISTENING",
	5: "PRE_MASTER", 6: "MASTER", 7: "PASSIVE", 8: "UNCALIBRATED", 9: "SLAVE",
}

// ptpServerSocket locates ptp4l's management socket: /var/run/pmc.<ifindex>
func ptpServerSocket() string {
	entries, _ := filepath.Glob("/var/run/pmc.*")
	for _, e := range entries {
		if _, err := strconv.Atoi(e[len("/var/run/pmc."):]); err == nil {
			return e
		}
	}
	return ""
}

// ptpMgmtRequest performs one GET round-trip and returns the response
// management TLV's dataField (the dataset bytes).
func ptpMgmtRequest(mid uint16) ([]byte, error) {
	server := ptpServerSocket()
	if server == "" {
		return nil, os.ErrNotExist
	}
	client := filepath.Join(os.TempDir(), "stageneth-pmc."+strconv.Itoa(os.Getpid()))
	os.Remove(client)
	defer os.Remove(client)

	conn, err := net.DialUnix("unixgram",
		&net.UnixAddr{Name: client},
		&net.UnixAddr{Name: server})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ptpSeq++
	msg := buildMgmtGet(mid, ptpSeq)
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return parseMgmtResponse(buf[:n], mid, ptpSeq)
}

// buildMgmtGet builds a PTPv2 management GET for the given managementId.
func buildMgmtGet(mid, seq uint16) []byte {
	b := make([]byte, 54)
	b[0] = ptpMsgManagement // transportSpecific(0) | messageType
	b[1] = 0x02             // reserved | versionPTP
	binary.BigEndian.PutUint16(b[2:4], 54)
	// bytes 4-19: domainNumber/reserved/flags/correctionField — zeros
	// sourcePortIdentity left zeroed; ptp4l replies by sequenceId anyway
	for i := 20; i < 28; i++ {
		b[i] = 0xff // clockIdentity 8×0xff (broadcast-ish, matches pmc)
	}
	binary.BigEndian.PutUint16(b[28:30], 0xffff)
	binary.BigEndian.PutUint16(b[30:32], seq)
	b[32] = 4 // controlField: management
	// management body
	for i := 34; i < 42; i++ {
		b[i] = 0xff // targetPortIdentity.clockIdentity
	}
	binary.BigEndian.PutUint16(b[42:44], 0xffff) // targetPortIdentity.portNumber
	b[44] = 0                                    // startingBoundaryHops
	b[45] = 0                                    // boundaryHops
	b[46] = ptpActionGet
	// managementTLV
	binary.BigEndian.PutUint16(b[48:50], tlvManagement)
	binary.BigEndian.PutUint16(b[50:52], 2) // lengthField (managementId only)
	binary.BigEndian.PutUint16(b[52:54], mid)
	return b
}

// parseMgmtResponse validates a management RESPONSE and returns the
// dataset bytes for the requested managementId.
func parseMgmtResponse(b []byte, mid, seq uint16) ([]byte, error) {
	if len(b) < 54 {
		return nil, errShort
	}
	if b[0]&0x0f != ptpMsgManagement {
		return nil, errProto
	}
	if binary.BigEndian.Uint16(b[30:32]) != seq {
		return nil, errProto
	}
	if b[46]&0x0f != ptpActionResponse {
		return nil, errProto
	}
	tlvType := binary.BigEndian.Uint16(b[48:50])
	tlvLen := int(binary.BigEndian.Uint16(b[50:52]))
	if tlvType != tlvManagement || 52+tlvLen > len(b) {
		return nil, errProto
	}
	dataField := b[52 : 52+tlvLen]
	if len(dataField) < 2 {
		return nil, errShort
	}
	got := binary.BigEndian.Uint16(dataField[0:2])
	// managementId comes first in a response dataField
	if got == mid {
		return dataField[2:], nil
	}
	return nil, errProto
}

var (
	errShort = errMsg("ptp: short frame")
	errProto = errMsg("ptp: bad response")
)

type errMsg string

func (e errMsg) Error() string { return string(e) }

// parsePortDataSet extracts portState (byte at offset 10).
func parsePortDataSet(b []byte) string {
	if len(b) < 11 {
		return "unknown"
	}
	if s, ok := portStateNames[b[10]]; ok {
		return s
	}
	return "unknown"
}

// parseCurrentDataSet extracts offsetFromMaster and meanPathDelay —
// both int64 scaled nanoseconds (TimeInterval), after stepsRemoved(u16).
func parseCurrentDataSet(b []byte) (offsetNs, delayNs float64) {
	if len(b) < 18 {
		return 0, 0
	}
	offsetNs = float64(int64(binary.BigEndian.Uint64(b[2:10])))
	delayNs = float64(int64(binary.BigEndian.Uint64(b[10:18])))
	return offsetNs, delayNs
}
