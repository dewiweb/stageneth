package mon

import (
	"encoding/binary"
	"testing"
)

func buildMgmtResponse(mid, seq uint16, dataset []byte) []byte {
	b := make([]byte, 54+len(dataset))
	b[0] = ptpMsgManagement
	b[1] = 0x02
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	binary.BigEndian.PutUint16(b[30:32], seq)
	b[32] = 4
	b[46] = ptpActionResponse
	binary.BigEndian.PutUint16(b[48:50], tlvManagement)
	binary.BigEndian.PutUint16(b[50:52], uint16(2+len(dataset)))
	binary.BigEndian.PutUint16(b[52:54], mid)
	copy(b[54:], dataset)
	return b
}

func TestMgmtResponseRoundTrip(t *testing.T) {
	ds := make([]byte, 20)
	ds[10] = 9 // portState SLAVE
	resp := buildMgmtResponse(midPortDataSet, 7, ds)
	got, err := parseMgmtResponse(resp, midPortDataSet, 7)
	if err != nil {
		t.Fatal(err)
	}
	if parsePortDataSet(got) != "SLAVE" {
		t.Fatalf("portState = %q", parsePortDataSet(got))
	}
}

func TestMgmtResponseWrongSeq(t *testing.T) {
	resp := buildMgmtResponse(midPortDataSet, 7, make([]byte, 20))
	if _, err := parseMgmtResponse(resp, midPortDataSet, 9); err == nil {
		t.Error("stale seq accepted")
	}
	if _, err := parseMgmtResponse(resp[:30], midPortDataSet, 7); err == nil {
		t.Error("short frame accepted")
	}
	if _, err := parseMgmtResponse(resp, midCurrentDataSet, 7); err == nil {
		t.Error("wrong mid accepted")
	}
}

func TestParseCurrentDataSet(t *testing.T) {
	ds := make([]byte, 18)
	binary.BigEndian.PutUint16(ds[0:2], 1)              // stepsRemoved
	binary.BigEndian.PutUint64(ds[2:10], uint64(42000)) // offset ns
	binary.BigEndian.PutUint64(ds[10:18], uint64(8000)) // delay ns
	off, del := parseCurrentDataSet(ds)
	if off != 42000 || del != 8000 {
		t.Errorf("off=%v del=%v", off, del)
	}
	if _, _ = parseCurrentDataSet(ds[:5]); true {
		// must not panic on truncation
	}
}

func TestPortStateNamesCoverage(t *testing.T) {
	ds := make([]byte, 11)
	for state, want := range map[byte]string{4: "LISTENING", 6: "MASTER", 9: "SLAVE", 8: "UNCALIBRATED"} {
		ds[10] = state
		if got := parsePortDataSet(ds); got != want {
			t.Errorf("state %d = %q want %q", state, got, want)
		}
	}
	ds[10] = 0xEE
	if got := parsePortDataSet(ds); got != "unknown" {
		t.Error("unknown state should map to unknown")
	}
}
