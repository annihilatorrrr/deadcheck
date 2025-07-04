package pd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/adamdecaf/deadcheck/internal/config"
	"github.com/adamdecaf/deadcheck/internal/provider/snooze"

	"github.com/PagerDuty/go-pagerduty"
	"github.com/moov-io/base/log"
	"github.com/moov-io/base/stime"
)

type Client interface {
	Setup(ctx context.Context, check config.Check) error
	CheckIn(ctx context.Context, check config.Check) (time.Time, error)

	setupService(ctx context.Context, check config.Check) (*pagerduty.Service, error)
}

func NewClient(logger log.Logger, conf *config.PagerDuty, timeService stime.TimeService) (Client, error) {
	if conf == nil {
		return nil, nil
	}

	cc := &client{
		logger:      logger,
		pdConfig:    *conf,
		timeService: timeService,
		underlying:  pagerduty.NewClient(conf.ApiKey),
	}
	if err := cc.ping(); err != nil {
		return nil, err
	}

	return cc, nil
}

type client struct {
	logger      log.Logger
	pdConfig    config.PagerDuty
	timeService stime.TimeService
	underlying  *pagerduty.Client
}

var _ Client = (&client{})

func (c *client) ping() error {
	ctx := context.Background()
	resp, err := c.underlying.ListAbilitiesWithContext(ctx)
	if err != nil {
		return fmt.Errorf("pagerduty list abilities: %w", err)
	}
	if len(resp.Abilities) <= 0 {
		return errors.New("pagerduty: missing abilities")
	}
	return nil
}

func (c *client) Setup(ctx context.Context, check config.Check) error {
	service, err := c.setupService(ctx, check)
	if err != nil {
		return fmt.Errorf("setup service: %w", err)
	}

	ep, err := c.findEscalationPolicy(ctx, escalationPolicySetup{
		id: c.pdConfig.EscalationPolicy,
	})
	if err != nil {
		return fmt.Errorf("finding escalation policy: %w", err)
	}

	// Find or create our ongoing incident
	inc, err := c.setupInitialIncident(ctx, service, ep)
	if err != nil {
		return fmt.Errorf("setup initial incident: %w", err)
	}

	logger := c.logger.With(log.Fields{
		"incident_id":  log.String(inc.ID),
		"service_id":   log.String(service.ID),
		"service_name": log.String(service.Name),
	})
	logger.Info().Logf("using incident %s on service %v", inc.ID, service.Name)

	now := c.timeService.Now()
	_, wait, err := snooze.Calculate(now, check.Schedule)
	if err != nil {
		return fmt.Errorf("calculating snooze: %w", err)
	}

	err = c.snoozeIncident(ctx, logger, inc, service, now, wait)
	if err != nil {
		return fmt.Errorf("snoozing incident %s for %s failed: %w", inc.ID, wait, err)
	}

	return nil
}

func (c *client) CheckIn(ctx context.Context, check config.Check) (time.Time, error) {
	service, err := c.setupService(ctx, check)
	if err != nil {
		return time.Time{}, fmt.Errorf("setup service: %w", err)
	}

	ep, err := c.findEscalationPolicy(ctx, escalationPolicySetup{
		id: c.pdConfig.EscalationPolicy,
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("finding escalation policy: %w", err)
	}

	// Find or create our ongoing incident
	inc, err := c.setupInitialIncident(ctx, service, ep)
	if err != nil {
		return time.Time{}, fmt.Errorf("setup initial incident: %w", err)
	}

	logger := c.logger.Info().With(log.Fields{
		"incident_id":  log.String(inc.ID),
		"service_id":   log.String(service.ID),
		"service_name": log.String(service.Name),
	})
	logger.Info().Logf("using incident %s on service %v", inc.ID, service.Name)

	// Easy way to calculate would be to find the remaining snooze and add that to now()
	// then calculate the next snooze.
	now := c.timeService.Now()

	scheduleTime, wait, err := snooze.Calculate(now, check.Schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("calculating snooze: %w", err)
	}

	// Only allow check-ins with the tolerance specified.
	//  e.g. If the tolerance is 5mins for a check-in expected at 4pm then only between 3:55pm and 4:05pm
	//       would check-ins be allowed.
	err = config.WithinTolerance(now, scheduleTime, check.Schedule)
	if err != nil {
		return time.Time{}, logger.Error().LogError(err).Err()
	}

	// future := now.Add(wait)
	_, wait, err = snooze.Calculate(scheduleTime, check.Schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("calculating second snooze: %w", err)
	}

	future := scheduleTime.Add(wait)
	logger.Info().Logf("snoozing incident %s until %v", inc.ID, future.Format(time.RFC3339))

	err = c.snoozeIncident(ctx, logger, inc, service, now, future.Sub(now))
	if err != nil {
		return time.Time{}, fmt.Errorf("snoozing incident %s for %s failed: %w", inc.ID, wait, err)
	}

	return future, nil
}
