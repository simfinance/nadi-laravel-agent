package enrollment

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"nadi-agent/internal/state"
	"nadi-agent/internal/version"
)

func ReadManifest(path string) (Manifest, error) {
	if strings.TrimSpace(path) == "" {
		return Manifest{}, errors.New("enrollment manifest path is required")
	}

	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open enrollment manifest: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode enrollment manifest: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, errors.New("enrollment manifest must contain exactly one JSON document")
		}
		return Manifest{}, fmt.Errorf("read enrollment manifest ending: %w", err)
	}

	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func BuildRequest(agentState state.State, token string, manifest Manifest) ([]byte, string, error) {
	token = strings.TrimSpace(token)
	if len(token) != 76 {
		return nil, "", errors.New("NADI_ENROLLMENT_TOKEN must contain exactly 76 characters")
	}
	if err := validateManifest(manifest); err != nil {
		return nil, "", err
	}

	requestID, err := newUUID()
	if err != nil {
		return nil, "", fmt.Errorf("generate enrollment request ID: %w", err)
	}

	hostname := strings.TrimSpace(agentState.Hostname)
	if hostname == "" || agentState.AgentInstanceID == "" {
		return nil, "", errors.New("Agent identity is incomplete")
	}

	osName, osVersion := readOSRelease()
	ipAddress, err := readServerIPv4Address()
	if err != nil {
		return nil, "", fmt.Errorf("read server IPv4 address: %w", err)
	}

	request := EnrollmentRequest{
		EnrollmentToken: token,
		RequestID:       requestID,
		Server: ServerInfo{
			Hostname:     hostname,
			IPAddress:    ipAddress,
			OSName:       osName,
			OSVersion:    osVersion,
			Architecture: runtime.GOARCH,
		},
		Agent: AgentInfo{
			AgentInstanceID: agentState.AgentInstanceID,
			AgentVersion:    version.Agent,
			ProtocolVersion: version.Protocol,
		},
		Applications: manifest.Applications,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, "", fmt.Errorf("encode Enrollment request: %w", err)
	}

	return body, requestID, nil
}

func readServerIPv4Address() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list network interfaces: %w", err)
	}

	defaultInterface := readDefaultRouteInterface()
	candidates := make([]net.Interface, 0, len(interfaces))
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}

		for _, address := range addresses {
			if ipv4 := usableIPv4(address); ipv4 != "" {
				candidates = append(candidates, networkInterface)
				break
			}
		}
	}

	if len(candidates) == 0 {
		return "", errors.New("no usable IPv4 address found on an active non-loopback interface")
	}

	selected := -1
	if defaultInterface != "" {
		for index, networkInterface := range candidates {
			if networkInterface.Name == defaultInterface {
				selected = index
				break
			}
		}
	}

	if selected == -1 {
		if len(candidates) != 1 {
			return "", errors.New("multiple active IPv4 interfaces found and default route interface could not be determined")
		}
		selected = 0
	}

	addresses, err := candidates[selected].Addrs()
	if err != nil {
		return "", fmt.Errorf("read addresses for interface %q: %w", candidates[selected].Name, err)
	}

	ipv4Addresses := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if ipv4 := usableIPv4(address); ipv4 != "" {
			ipv4Addresses = append(ipv4Addresses, ipv4)
		}
	}
	sort.Strings(ipv4Addresses)
	if len(ipv4Addresses) == 0 {
		return "", fmt.Errorf("interface %q has no usable IPv4 address", candidates[selected].Name)
	}

	return ipv4Addresses[0], nil
}

func usableIPv4(address net.Addr) string {
	var ip net.IP
	switch value := address.(type) {
	case *net.IPNet:
		ip = value.IP
	case *net.IPAddr:
		ip = value.IP
	default:
		return ""
	}

	ipv4 := ip.To4()
	if ipv4 == nil || ipv4.IsLoopback() || ipv4.IsLinkLocalUnicast() || ipv4.IsUnspecified() || ipv4.IsMulticast() {
		return ""
	}

	return ipv4.String()
}

func readDefaultRouteInterface() string {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return ""
	}

	selectedInterface := ""
	selectedMetric := int(^uint(0) >> 1)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[1] != "00000000" {
			continue
		}

		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil || flags&1 == 0 {
			continue
		}

		metric, err := strconv.Atoi(fields[6])
		if err != nil || metric >= selectedMetric {
			continue
		}

		selectedInterface = fields[0]
		selectedMetric = metric
	}

	return selectedInterface
}

func validateManifest(manifest Manifest) error {
	if len(manifest.Applications) < 1 || len(manifest.Applications) > 20 {
		return errors.New("enrollment manifest must contain between 1 and 20 applications")
	}

	keys := make(map[string]struct{}, len(manifest.Applications))
	for index, application := range manifest.Applications {
		if strings.TrimSpace(application.ApplicationKey) == "" || len(application.ApplicationKey) > 100 {
			return fmt.Errorf("applications[%d].application_key is required and must not exceed 100 characters", index)
		}
		if _, exists := keys[application.ApplicationKey]; exists {
			return fmt.Errorf("applications[%d].application_key is duplicated", index)
		}
		keys[application.ApplicationKey] = struct{}{}

		if strings.TrimSpace(application.ApplicationName) == "" || len(application.ApplicationName) > 255 {
			return fmt.Errorf("applications[%d].application_name is required and must not exceed 255 characters", index)
		}
		if strings.TrimSpace(application.Environment) == "" || len(application.Environment) > 50 {
			return fmt.Errorf("applications[%d].environment is required and must not exceed 50 characters", index)
		}
		if len(application.FrameworkVersion) > 50 {
			return fmt.Errorf("applications[%d].framework_version exceeds 50 characters", index)
		}
		if len(application.PHPVersion) > 30 {
			return fmt.Errorf("applications[%d].php_version exceeds 30 characters", index)
		}
		if strings.TrimSpace(application.BaseURL) == "" || len(application.BaseURL) > 2048 {
			return fmt.Errorf("applications[%d].base_url is required and must not exceed 2048 characters", index)
		}

		parsedURL, err := url.ParseRequestURI(application.BaseURL)
		if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
			return fmt.Errorf("applications[%d].base_url must be an absolute HTTPS URL", index)
		}
	}

	return nil
}

func readOSRelease() (string, string) {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS, ""
	}

	values := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[key] = strings.Trim(value, "\"'")
	}

	name := values["PRETTY_NAME"]
	if name == "" {
		name = values["NAME"]
	}
	if name == "" {
		name = runtime.GOOS
	}

	return name, values["VERSION_ID"]
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}

	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	encoded := make([]byte, 32)
	hex.Encode(encoded, value[:])

	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}
