package pd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/adamdecaf/deadcheck/internal/config"

	"github.com/moov-io/base"
	"github.com/moov-io/base/log"
	"github.com/moov-io/base/stime"
	"github.com/stretchr/testify/require"
)

func TestService__Setup(t *testing.T) {
	ctx := context.Background()

	conf := config.Check{
		ID:   base.ID(),
		Name: makeServiceName(t),
		Schedule: config.ScheduleConfig{
			Weekdays: &config.PartialDay{
				Timezone:  "America/New_York",
				Times:     []string{"12:07"},
				Tolerance: "5h25m",
			},
		},
	}
	pdc := newTestClient(t)

	service, err := pdc.setupService(ctx, conf)
	require.NoError(t, err)
	t.Cleanup(func() {
		pdc.deleteService(ctx, service)
	})

	t.Logf("setup service %v named %v", service.ID, service.Name)

	// Verify the service is in maintenance mode
	found, err := pdc.findService(ctx, conf.Name)
	require.NoError(t, err)
	require.Equal(t, service.ID, found.ID)
}

func TestService_SnoozedIncident(t *testing.T) {
	skipInCI(t) // This test creates real alerts, so don't run it in CI

	ctx := context.Background()
	logger := log.NewTestLogger()

	conf := config.Check{
		ID:   base.ID(),
		Name: makeServiceName(t),
	}
	pdc := newTestClient(t)

	service, err := pdc.setupService(ctx, conf)
	require.NoError(t, err)
	t.Cleanup(func() {
		pdc.deleteService(ctx, service)
	})

	t.Logf("setup service %v named %v", service.ID, service.Name)

	// Create a new escalation policy with nothing routed
	ep, err := pdc.findEscalationPolicy(ctx, escalationPolicySetup{
		id: defaultEscalationPolicy,
	})
	require.NoError(t, err)

	// Create an incident
	inc, err := pdc.setupInitialIncident(ctx, service, ep)
	require.NoError(t, err)

	t.Logf("created incident %v escalating to %v", inc.ID, ep.Name)

	timeService := stime.NewSystemTimeService()
	now := timeService.Now()
	err = pdc.snoozeIncident(ctx, logger, inc, service, now, time.Hour)
	require.NoError(t, err)

	inc, err = pdc.setupInitialIncident(ctx, service, ep)
	require.NoError(t, err)

	// Resolve incident
	err = pdc.resolveIncident(ctx, inc)
	require.NoError(t, err)
}

func makeServiceName(t *testing.T) string {
	return fmt.Sprintf("%s_%d", t.Name(), time.Now().In(time.UTC).Unix())
}
