package app

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	"nadi-agent/internal/activation"
	"nadi-agent/internal/appmonitor"
	"nadi-agent/internal/appspool"
	"nadi-agent/internal/config"
	"nadi-agent/internal/control"
	"nadi-agent/internal/identity"
	"nadi-agent/internal/servermetrics"
	"nadi-agent/internal/spool"
	"nadi-agent/internal/state"
	"nadi-agent/internal/telemetry"
)

type App struct {
	config             config.Config
	logger             *slog.Logger
	controlHTTP        *control.Client
	serverMetrics      *servermetrics.Collector
	applicationMetrics *appmonitor.Collector
	telemetrySpool     *spool.Store
	applicationSpool   *appspool.Store
	telemetrySender    *telemetry.Sender
}

func New(cfg config.Config, logger *slog.Logger) *App {
	return &App{
		config: cfg,
		logger: logger,
		controlHTTP: control.NewClient(
			cfg.CentralURL,
			cfg.AgentToken,
			cfg.HTTPTimeout,
		),
		serverMetrics:      servermetrics.NewCollector(),
		applicationMetrics: appmonitor.NewCollector(cfg.HTTPTimeout),
	}
}

func (a *App) Run(ctx context.Context) error {
	store, err := state.Open(a.config.StateDir)
	if err != nil {
		return fmt.Errorf("initialize local state: %w", err)
	}
	defer store.Close()

	telemetrySpool, err := spool.Open(filepath.Join(a.config.StateDir, "spool"), spool.DefaultMaxBytes, spool.DefaultMaxRecords)
	if err != nil {
		return fmt.Errorf("initialize telemetry spool: %w", err)
	}
	a.telemetrySpool = telemetrySpool

	applicationSpool, err := appspool.Open(filepath.Join(a.config.StateDir, "spool", "applications"), appspool.DefaultMaxBytes, appspool.DefaultMaxRecords)
	if err != nil {
		return fmt.Errorf("initialize per-application telemetry spool: %w", err)
	}
	a.applicationSpool = applicationSpool

	telemetrySender, err := telemetry.NewSender(
		a.config.CentralURL,
		filepath.Join(a.config.StateDir, "telemetry-delivery"),
		a.config.HTTPTimeout,
	)
	if err != nil {
		return fmt.Errorf("initialize Central telemetry sender: %w", err)
	}
	a.telemetrySender = telemetrySender

	agentState, err := identity.Ensure(store)
	if err != nil {
		return fmt.Errorf("initialize agent identity: %w", err)
	}

	a.logger.Info(
		"NADI Agent core started",
		"agent_instance_id", agentState.AgentInstanceID,
		"hostname", agentState.Hostname,
		"enrollment_status", agentState.EnrollmentStatus,
		"central_url", a.config.CentralURL,
		"poll_interval_seconds", int(a.config.PollInterval.Seconds()),
	)
	a.logger.Info(
		"Agent identity and local state initialized",
		"state_dir", a.config.StateDir,
	)

	if agentState.CredentialRotation != nil {
		if a.config.AgentToken != "" {
			return fmt.Errorf("NADI_AGENT_TOKEN override is incompatible with a pending credential rotation")
		}
		if err := a.resumeCredentialRotation(ctx, store, &agentState); err != nil {
			a.logger.Warn("Pending credential rotation will be resumed before Control Poll", "error", err)
		}
		agentState, err = store.Load()
		if err != nil {
			return fmt.Errorf("reload Agent state after credential rotation recovery: %w", err)
		}
	}

	token := a.config.AgentToken
	if token == "" {
		token = agentState.AgentCredential
	}

	if token != "" && a.config.AgentToken == "" && !agentState.AgentCredentialExpiresAt.IsZero() && !agentState.AgentCredentialExpiresAt.After(time.Now().UTC()) {
		a.logger.Warn("Persisted Agent credential has expired; Control Poll remains disabled")
		token = ""
	}

	if token == "" {
		a.logger.Warn("Control Poll is waiting for an active Agent credential")
		<-ctx.Done()
		a.logger.Info("NADI Agent core is shutting down")
		a.controlHTTP.HTTP.CloseIdleConnections()
		return nil
	}

	a.controlHTTP.SetToken(token)
	a.controlHTTP.SetPollURL(agentState.ControlPollURL)
	if agentState.PollIntervalSeconds > 0 {
		a.config.PollInterval = time.Duration(agentState.PollIntervalSeconds) * time.Second
	}

	return a.runControlPoll(ctx, store, agentState)
}

func (a *App) runControlPoll(ctx context.Context, store *state.Store, agentState state.State) error {
	backoff := time.Second
	const maximumBackoff = 5 * time.Minute

	for {
		if ctx.Err() != nil {
			break
		}

		currentState, loadErr := store.Load()
		if loadErr == nil && currentState.CredentialRotation != nil {
			if err := a.resumeCredentialRotation(ctx, store, &currentState); err != nil {
				delay := jitter(backoff)
				a.logger.Warn("Credential rotation ACK remains pending; Control Poll is paused", "error", err, "retry_in_seconds", int(delay.Seconds()))
				if !wait(ctx, delay) {
					break
				}
				backoff *= 2
				if backoff > maximumBackoff {
					backoff = maximumBackoff
				}
				continue
			}
			backoff = time.Second
		}

		if loadErr == nil && len(currentState.PendingAcknowledgements) > 0 {
			if err := a.flushPendingAcknowledgements(ctx, store, &currentState); err != nil {
				a.logger.Warn("Pending Activation Grant acknowledgement retry failed", "error", err)
			}
		}

		pollData, err := a.controlHTTP.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}

			delay := jitter(backoff)
			a.logger.Error(
				"Central Control Poll failed; retry scheduled",
				"error", err,
				"retry_in_seconds", int(delay.Seconds()),
			)
			if !wait(ctx, delay) {
				break
			}

			backoff *= 2
			if backoff > maximumBackoff {
				backoff = maximumBackoff
			}
			continue
		}

		backoff = time.Second
		if pollData.Agent.ID != agentState.CentralAgentID || pollData.Agent.AgentInstanceID != agentState.AgentInstanceID {
			a.logger.Error("Control Poll identity mismatch; response ignored", "central_agent_id", pollData.Agent.ID, "agent_instance_id", pollData.Agent.AgentInstanceID)
			if !wait(ctx, jitter(backoff)) {
				break
			}
			continue
		}

		a.logger.Info(
			"Central Control Poll succeeded",
			"central_agent_id", pollData.Agent.ID,
			"applications_received", len(pollData.Applications),
			"commands_received", len(pollData.Commands),
		)

		currentState, loadErr = store.Load()
		if loadErr != nil {
			a.logger.Error("Unable to reload local state before Activation Grant processing", "error", loadErr)
		} else if err := persistApplicationControlState(store, &currentState, pollData); err != nil {
			a.logger.Error("Unable to persist Central application control state; monitoring remains disabled", "error", err)
		} else {
			if err := a.processActivationGrants(ctx, store, currentState, pollData); err != nil {
				a.logger.Error("Activation Grant processing failed; monitoring remains disabled", "error", err)
			}

			latestState, stateErr := store.Load()
			if stateErr != nil {
				a.logger.Error("Unable to reload local state for Runtime Activation Gate; monitoring remains disabled", "error", stateErr)
			} else {
				serverMetricsAllowed, gateDecisions := a.evaluateRuntimeGates(latestState, pollData, time.Now().UTC())
				a.collectApplicationMetrics(ctx, latestState, gateDecisions)
				if serverMetricsAllowed {
					a.collectServerMetrics(latestState)
				} else {
					a.serverMetrics.Reset()
					if err := a.telemetrySpool.Clear(); err != nil {
						a.logger.Error("Unable to clear telemetry spool while Runtime Activation Gate is closed", "error", err)
					}
				}
				if err := a.deliverTelemetry(ctx, latestState, serverMetricsAllowed, gateDecisions); err != nil {
					a.logger.Warn("Central telemetry delivery remains pending; local spool is retained", "error", err)
				}
			}
		}

		if len(pollData.Commands) > 0 {
			a.logger.Warn(
				"Central delivered commands; command execution is disabled in this Agent milestone",
				"commands_received", len(pollData.Commands),
			)
		}

		if !wait(ctx, a.config.PollInterval) {
			break
		}
	}

	a.logger.Info("NADI Agent core is shutting down")
	a.controlHTTP.HTTP.CloseIdleConnections()
	return nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func jitter(duration time.Duration) time.Duration {
	factor := 0.5 + rand.Float64()
	return time.Duration(float64(duration) * factor)
}

func (a *App) collectServerMetrics(agentState state.State) {
	snapshot, err := a.serverMetrics.Collect(time.Now().UTC())
	if err != nil {
		a.logger.Error("Server metrics collection failed", "server_id", agentState.CentralServerID, "error", err)
		return
	}

	record, err := a.telemetrySpool.Append(agentState.CentralServerID, agentState.CentralAgentID, snapshot)
	if err != nil {
		a.logger.Error("Server metrics could not be persisted to the bounded telemetry spool", "server_id", agentState.CentralServerID, "error", err)
		return
	}

	a.logger.Info(
		"Server metrics collected and queued locally",
		"event_id", record.EventID,
		"server_id", record.ServerID,
		"timestamp", snapshot.Timestamp,
		"spool_event_type", record.EventType,
	)
}

func (a *App) collectApplicationMetrics(
	ctx context.Context,
	agentState state.State,
	decisions map[string]activation.GateDecision,
) {
	const maximumConcurrentApplications = 4

	semaphore := make(chan struct{}, maximumConcurrentApplications)
	var workers sync.WaitGroup

collectionLoop:
	for _, application := range agentState.Applications {
		decision, found := decisions[application.ApplicationID]
		if !found || !decision.Enabled {
			if err := a.applicationSpool.Clear(application.ApplicationID); err != nil {
				a.logger.Error("Unable to clear application spool while Activation Gate is closed", "application_id", application.ApplicationID, "error", err)
			}
			continue
		}

		if !hasApplicationMonitoringCapability(decision.Capabilities) {
			if err := a.applicationSpool.Clear(application.ApplicationID); err != nil {
				a.logger.Error("Unable to clear application spool without an application monitoring capability", "application_id", application.ApplicationID, "error", err)
			}
			continue
		}

		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			break collectionLoop
		}

		workers.Add(1)
		go func(application state.EnrolledApplication, decision activation.GateDecision) {
			defer workers.Done()
			defer func() {
				<-semaphore
			}()

			now := time.Now().UTC()
			snapshot := a.applicationMetrics.Collect(
				ctx,
				application.Endpoint,
				application.ApplicationCredential,
				decision.Capabilities,
				now,
			)
			record, err := a.applicationSpool.Append(
				agentState.CentralServerID,
				agentState.CentralAgentID,
				application.ApplicationID,
				snapshot,
			)
			if err != nil {
				a.logger.Error("Application metrics could not be persisted to the isolated spool", "application_id", application.ApplicationID, "error", err)
				return
			}

			a.logger.Info(
				"Application metrics collected and queued locally",
				"event_id", record.EventID,
				"application_id", record.ApplicationID,
				"timestamp", snapshot.Timestamp,
				"health_status", snapshot.Health.Status,
				"info_status", snapshot.Info.Status,
				"database_status", snapshot.Database.Status,
				"metrics_status", snapshot.Metrics.Status,
				"queue_status", snapshot.Queue.Status,
				"scheduler_status", snapshot.Scheduler.Status,
			)
		}(application, decision)
	}

	workers.Wait()
}

func hasApplicationMonitoringCapability(capabilities []string) bool {
	return hasCapability(capabilities, appmonitor.CapabilityApplicationMetrics) ||
		hasCapability(capabilities, appmonitor.CapabilityDatabaseMonitoring) ||
		hasCapability(capabilities, appmonitor.CapabilityQueueMonitoring) ||
		hasCapability(capabilities, appmonitor.CapabilitySchedulerMonitoring)
}
