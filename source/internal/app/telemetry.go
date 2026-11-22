package app

import (
	"context"
	"encoding/json"
	"fmt"

	"nadi-agent/internal/activation"
	"nadi-agent/internal/state"
	"nadi-agent/internal/telemetry"
)

func (a *App) deliverTelemetry(
	ctx context.Context,
	agentState state.State,
	serverMetricsAllowed bool,
	decisions map[string]activation.GateDecision,
) error {
	pendingSources, hasPending, err := a.telemetrySender.PendingSources()
	if err != nil {
		return fmt.Errorf("inspect pending Central telemetry batch: %w", err)
	}
	if hasPending {
		for _, source := range pendingSources {
			if source.Kind == "server" && !serverMetricsAllowed {
				if err := a.telemetrySender.DiscardPending(); err != nil {
					return fmt.Errorf("discard telemetry batch after server Activation Gate closed: %w", err)
				}
				return nil
			}
			if source.Kind == "application" {
				decision, found := decisions[source.ApplicationID]
				if !found || !decision.Enabled || !hasApplicationMonitoringCapability(decision.Capabilities) {
					if err := a.telemetrySender.DiscardPending(); err != nil {
						return fmt.Errorf("discard telemetry batch after application Activation Gate closed: %w", err)
					}
					return nil
				}
			}
		}
	}

	inputs := make([]telemetry.InputRecord, 0, 500)

	serverRecords, err := a.telemetrySpool.ReadBatch(telemetry.MaximumBatchRecords)
	if err != nil {
		return fmt.Errorf("read server telemetry spool: %w", err)
	}
	for _, record := range serverRecords {
		data, err := json.Marshal(record.Payload)
		if err != nil {
			return fmt.Errorf("encode server metrics snapshot: %w", err)
		}
		inputs = append(inputs, telemetry.InputRecord{
			Stream:      "server_metrics",
			ObservedAt:  record.Timestamp,
			CollectedAt: record.Timestamp,
			Data:        data,
			Source: telemetry.Source{
				Kind:    "server",
				EventID: record.EventID,
			},
		})
	}

	for _, application := range agentState.Applications {
		remaining := telemetry.MaximumBatchRecords - len(inputs)
		if remaining <= 0 {
			break
		}
		applicationRecords, err := a.applicationSpool.ReadBatch(application.ApplicationID, remaining)
		if err != nil {
			return fmt.Errorf("read application telemetry spool for %s: %w", application.ApplicationID, err)
		}
		for _, record := range applicationRecords {
			data, err := json.Marshal(record.Payload)
			if err != nil {
				return fmt.Errorf("encode application metrics snapshot: %w", err)
			}
			applicationID := record.ApplicationID
			inputs = append(inputs, telemetry.InputRecord{
				Stream:        "application_metrics",
				ObservedAt:    record.Timestamp,
				CollectedAt:   record.Timestamp,
				ApplicationID: &applicationID,
				Data:          data,
				Source: telemetry.Source{
					Kind:          "application",
					ApplicationID: record.ApplicationID,
					EventID:       record.EventID,
				},
			})
		}
	}

	acknowledged, err := a.telemetrySender.Send(
		ctx,
		agentState.CentralAgentID,
		agentState.AgentInstanceID,
		a.controlHTTP.Token,
		inputs,
	)
	if err != nil {
		return err
	}
	if len(acknowledged) == 0 {
		return nil
	}

	serverEventIDs := make([]string, 0, len(acknowledged))
	applicationEventIDs := make(map[string][]string)
	for _, source := range acknowledged {
		switch source.Kind {
		case "server":
			serverEventIDs = append(serverEventIDs, source.EventID)
		case "application":
			applicationEventIDs[source.ApplicationID] = append(applicationEventIDs[source.ApplicationID], source.EventID)
		default:
			return fmt.Errorf("unknown acknowledged telemetry source kind %q", source.Kind)
		}
	}

	if err := a.telemetrySpool.Acknowledge(serverEventIDs); err != nil {
		return fmt.Errorf("acknowledge Central-accepted server telemetry: %w", err)
	}
	for applicationID, eventIDs := range applicationEventIDs {
		if err := a.applicationSpool.Acknowledge(applicationID, eventIDs); err != nil {
			return fmt.Errorf("acknowledge Central-accepted telemetry for application %s: %w", applicationID, err)
		}
	}
	if err := a.telemetrySender.Complete(); err != nil {
		return fmt.Errorf("finalize Central-accepted telemetry batch: %w", err)
	}

	a.logger.Info(
		"Central accepted telemetry batch; acknowledged local spool records",
		"record_count", len(acknowledged),
	)
	return nil
}
