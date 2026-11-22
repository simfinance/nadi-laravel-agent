package enrollment

type Manifest struct {
	Applications []ApplicationInput `json:"applications"`
}

type ApplicationInput struct {
	ApplicationKey   string `json:"application_key"`
	ApplicationName  string `json:"application_name"`
	Environment      string `json:"environment"`
	FrameworkVersion string `json:"framework_version,omitempty"`
	PHPVersion       string `json:"php_version,omitempty"`
	BaseURL          string `json:"base_url"`
}

type EnrollmentRequest struct {
	EnrollmentToken string             `json:"enrollment_token"`
	RequestID       string             `json:"request_id"`
	Server          ServerInfo         `json:"server"`
	Agent           AgentInfo          `json:"agent"`
	Applications    []ApplicationInput `json:"applications"`
}

type ServerInfo struct {
	Hostname     string `json:"hostname"`
	IPAddress    string `json:"ip_address,omitempty"`
	OSName       string `json:"os_name,omitempty"`
	OSVersion    string `json:"os_version,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

type AgentInfo struct {
	AgentInstanceID string `json:"agent_instance_id"`
	AgentVersion    string `json:"agent_version"`
	ProtocolVersion string `json:"protocol_version"`
}

type Response struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Data    ResponseData `json:"data"`
}

type ResponseData struct {
	ServerID                 string                `json:"server_id"`
	AgentID                  string                `json:"agent_id"`
	AgentInstanceID          string                `json:"agent_instance_id"`
	AgentStatus              string                `json:"agent_status"`
	AgentCredential          string                `json:"agent_credential"`
	AgentCredentialExpiresAt string                `json:"agent_credential_expires_at"`
	Control                  ControlInfo           `json:"control"`
	Applications             []ApplicationResponse `json:"applications"`
}

type ControlInfo struct {
	PollURL             string `json:"poll_url"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
}

type ApplicationResponse struct {
	ApplicationID         string `json:"application_id"`
	ApplicationKey        string `json:"application_key"`
	ApplicationName       string `json:"application_name"`
	Environment           string `json:"environment"`
	RegistrationID        string `json:"registration_id"`
	RegistrationStatus    string `json:"registration_status"`
	ActivationStatus      string `json:"activation_status"`
	MonitoringStatus      string `json:"monitoring_status"`
	Endpoint              string `json:"endpoint"`
	ApplicationCredential string `json:"application_credential"`
}
