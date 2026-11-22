package servermetrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type NetworkStats struct {
	Interface       string `json:"interface"`
	ReceiveBytes    uint64 `json:"receive_bytes"`
	ReceivePackets  uint64 `json:"receive_packets"`
	ReceiveErrors   uint64 `json:"receive_errors"`
	ReceiveDropped  uint64 `json:"receive_dropped"`
	TransmitBytes   uint64 `json:"transmit_bytes"`
	TransmitPackets uint64 `json:"transmit_packets"`
	TransmitErrors  uint64 `json:"transmit_errors"`
	TransmitDropped uint64 `json:"transmit_dropped"`
}

func collectNetworkStats(path string) ([]NetworkStats, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read network statistics: %w", err)
	}

	return parseNetworkStats(content)
}

func parseNetworkStats(content []byte) ([]NetworkStats, error) {
	networks := make([]NetworkStats, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, ":") {
			continue
		}

		separator := strings.LastIndex(line, ":")
		interfaceName := strings.TrimSpace(line[:separator])
		if interfaceName == "" || interfaceName == "lo" {
			continue
		}

		fields := strings.Fields(line[separator+1:])
		if len(fields) < 16 {
			return nil, fmt.Errorf("network statistics line %d has fewer than 16 counters", lineNumber)
		}

		counters := make([]uint64, 16)
		for index := range counters {
			value, err := strconv.ParseUint(fields[index], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid network counter for interface %s: %w", interfaceName, err)
			}
			counters[index] = value
		}

		networks = append(networks, NetworkStats{
			Interface:       interfaceName,
			ReceiveBytes:    counters[0],
			ReceivePackets:  counters[1],
			ReceiveErrors:   counters[2],
			ReceiveDropped:  counters[3],
			TransmitBytes:   counters[8],
			TransmitPackets: counters[9],
			TransmitErrors:  counters[10],
			TransmitDropped: counters[11],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan network statistics: %w", err)
	}

	return networks, nil
}
