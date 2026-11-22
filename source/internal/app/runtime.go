package app

import (
	"fmt"
	"time"

	"nadi-agent/internal/activation"
	"nadi-agent/internal/control"
	"nadi-agent/internal/state"
)

func syncApplicationControlState(current *state.State, pollData control.PollData) {
	applicationsByID := make(map[string]control.Application, len(pollData.Applications))
	for _, application := range pollData.Applications {
		applicationsByID[application.ApplicationID] = application
	}

	for index := range current.Applications {
		polled, found := applicationsByID[current.Applications[index].ApplicationID]
		if !found {
			current.Applications[index].RegistrationStatus = "unknown"
			current.Applications[index].ActivationStatus = "unknown"
			current.Applications[index].MonitoringStatus = "unknown"
			continue
		}

		current.Applications[index].RegistrationStatus = polled.RegistrationStatus
		current.Applications[index].ActivationStatus = polled.ActivationStatus
		current.Applications[index].MonitoringStatus = polled.MonitoringStatus
	}
}

func (a *App) evaluateRuntimeGates(
	current state.State,
	pollData control.PollData,
	now time.Time,
) (bool, map[string]activation.GateDecision) {
	if err := activation.ValidateAgentMonitoringState(pollData.Agent); err != nil {
		a.logger.Warn("Runtime Activation Gate is closed for all applications", "reason", err)
		return false, map[string]activation.GateDecision{}
	}

	verifier, err := activation.NewVerifier(a.config.ActivationPublicKeyPath, a.config.ActivationKeyID)
	if err != nil {
		a.logger.Error("Runtime Activation Gate could not initialize grant verifier; monitoring remains disabled", "error", err)
		return false, map[string]activation.GateDecision{}
	}

	polledApplications := make(map[string]*control.Application, len(pollData.Applications))
	for index := range pollData.Applications {
		application := &pollData.Applications[index]
		polledApplications[application.ApplicationID] = application
	}

	serverMetricsAllowed := false
	decisions := make(map[string]activation.GateDecision, len(current.Applications))

	for _, application := range current.Applications {
		decision := verifier.Evaluate(
			application,
			polledApplications[application.ApplicationID],
			current,
			now,
		)

		decisions[application.ApplicationID] = decision
		if !decision.Enabled {
			a.logger.Info(
				"Runtime Activation Gate closed; collectors must remain stopped",
				"application_id", application.ApplicationID,
				"reason", decision.Reason,
			)
			continue
		}

		a.logger.Info(
			"Runtime Activation Gate open",
			"application_id", application.ApplicationID,
			"grant_id", decision.GrantID,
			"capabilities", decision.Capabilities,
		)

		if hasCapability(decision.Capabilities, "server_metrics") {
			serverMetricsAllowed = true
		}
	}

	return serverMetricsAllowed, decisions
}

func hasCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}

	return false
}

func persistApplicationControlState(store *state.Store, current *state.State, pollData control.PollData) error {
	syncApplicationControlState(current, pollData)
	if err := store.Save(*current); err != nil {
		return fmt.Errorf("persist latest Central application control state: %w", err)
	}

	return nil
}
