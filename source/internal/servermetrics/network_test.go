package servermetrics

import "testing"

func TestParseNetworkStats(t *testing.T) {
	content := []byte(`Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 100 2 0 0 0 0 0 0 100 2 0 0 0 0 0 0
  ens160: 123456 1000 2 3 0 0 0 0 654321 900 4 5 0 0 0 0
`)

	networks, err := parseNetworkStats(content)
	if err != nil {
		t.Fatalf("parse network statistics: %v", err)
	}

	if len(networks) != 1 {
		t.Fatalf("expected one non-loopback interface, got %d", len(networks))
	}

	got := networks[0]
	if got.Interface != "ens160" {
		t.Fatalf("expected ens160, got %q", got.Interface)
	}
	if got.ReceiveBytes != 123456 || got.ReceivePackets != 1000 || got.ReceiveErrors != 2 || got.ReceiveDropped != 3 {
		t.Fatalf("unexpected receive counters: %+v", got)
	}
	if got.TransmitBytes != 654321 || got.TransmitPackets != 900 || got.TransmitErrors != 4 || got.TransmitDropped != 5 {
		t.Fatalf("unexpected transmit counters: %+v", got)
	}
}

func TestParseNetworkStatsRejectsIncompleteCounters(t *testing.T) {
	_, err := parseNetworkStats([]byte("ens160: 1 2 3\n"))
	if err == nil {
		t.Fatal("expected incomplete network counters to be rejected")
	}
}

func TestParseNetworkStatsAllowsNoNonLoopbackInterfaces(t *testing.T) {
	networks, err := parseNetworkStats([]byte("lo: 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16\n"))
	if err != nil {
		t.Fatalf("parse loopback-only network statistics: %v", err)
	}
	if len(networks) != 0 {
		t.Fatalf("expected no non-loopback interfaces, got %d", len(networks))
	}
}
