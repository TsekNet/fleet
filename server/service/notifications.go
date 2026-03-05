package service

import (
	"context"
	"encoding/json"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	hostctx "github.com/fleetdm/fleet/v4/server/contexts/host"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

////////////////////////////////////////////////////////////////////////////////
// Batch Set Notifications (user-authenticated, for GitOps)
////////////////////////////////////////////////////////////////////////////////

type batchSetNotificationsRequest struct {
	TeamID        *uint                        `json:"-" query:"team_id,optional"`
	TeamName      *string                      `json:"-" query:"team_name,optional"`
	DryRun        bool                         `json:"-" query:"dry_run,optional"`
	Notifications []fleet.NotificationPayload  `json:"notifications"`
}

type batchSetNotificationsResponse struct {
	Notifications []fleet.NotificationResponse `json:"notifications"`
	Err           error                        `json:"error,omitempty"`
}

func (r batchSetNotificationsResponse) Error() error { return r.Err }

func batchSetNotificationsEndpoint(ctx context.Context, request interface{}, svc fleet.Service) (fleet.Errorer, error) {
	req := request.(*batchSetNotificationsRequest)
	list, err := svc.BatchSetNotifications(ctx, req.TeamID, req.TeamName, req.Notifications, req.DryRun)
	if err != nil {
		return batchSetNotificationsResponse{Err: err}, nil
	}
	return batchSetNotificationsResponse{Notifications: list}, nil
}

func (svc *Service) BatchSetNotifications(ctx context.Context, maybeTmID *uint, maybeTmName *string, payloads []fleet.NotificationPayload, dryRun bool) ([]fleet.NotificationResponse, error) {
	if maybeTmID != nil && maybeTmName != nil {
		svc.authz.SkipAuthorization(ctx)
		return nil, ctxerr.Wrap(ctx, fleet.NewInvalidArgumentError("team_name", "cannot specify both team_id and team_name"))
	}

	var teamID *uint
	if maybeTmID != nil || maybeTmName != nil {
		team, err := svc.EnterpriseOverrides.TeamByIDOrName(ctx, maybeTmID, maybeTmName)
		if err != nil {
			if dryRun && fleet.IsNotFound(err) {
				svc.authz.SkipAuthorization(ctx)
				return nil, nil
			}
			svc.authz.SkipAuthorization(ctx)
			return nil, err
		}
		teamID = &team.ID
	}

	if err := svc.authz.Authorize(ctx, &fleet.Notification{TeamID: teamID}, fleet.ActionWrite); err != nil {
		return nil, ctxerr.Wrap(ctx, err)
	}

	if dryRun {
		return nil, nil
	}

	notifPayloads := make([]*fleet.NotificationPayload, len(payloads))
	for i := range payloads {
		notifPayloads[i] = &payloads[i]
	}
	return svc.ds.BatchSetNotifications(ctx, teamID, notifPayloads)
}

////////////////////////////////////////////////////////////////////////////////
// Get Orbit Notification Config (orbit-authenticated)
////////////////////////////////////////////////////////////////////////////////

type orbitGetNotificationConfigRequest struct {
	OrbitNodeKey   string `json:"orbit_node_key"`
	NotificationID string `json:"notification_id"`
}

func (r *orbitGetNotificationConfigRequest) setOrbitNodeKey(nodeKey string) {
	r.OrbitNodeKey = nodeKey
}

func (r *orbitGetNotificationConfigRequest) orbitHostNodeKey() string {
	return r.OrbitNodeKey
}

type orbitGetNotificationConfigResponse struct {
	Err    error           `json:"error,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
}

func (r orbitGetNotificationConfigResponse) Error() error { return r.Err }

func getOrbitNotificationConfigEndpoint(ctx context.Context, request interface{}, svc fleet.Service) (fleet.Errorer, error) {
	req := request.(*orbitGetNotificationConfigRequest)
	config, err := svc.GetOrbitNotificationConfig(ctx, req.NotificationID)
	if err != nil {
		return orbitGetNotificationConfigResponse{Err: err}, nil
	}
	return orbitGetNotificationConfigResponse{Config: config}, nil
}

func (svc *Service) GetOrbitNotificationConfig(ctx context.Context, notificationID string) (json.RawMessage, error) {
	svc.authz.SkipAuthorization(ctx)

	host, ok := hostctx.FromContext(ctx)
	if !ok {
		return nil, fleet.OrbitError{Message: "internal error: missing host from request context"}
	}

	notif, err := svc.ds.GetNotificationByNotificationID(ctx, notificationID, host.TeamID)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "get notification template")
	}

	contents, err := svc.ds.GetNotificationContents(ctx, notif.NotificationContentID)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "get notification contents")
	}

	return json.RawMessage(contents), nil
}

////////////////////////////////////////////////////////////////////////////////
// Post Orbit Notification Result (orbit-authenticated)
////////////////////////////////////////////////////////////////////////////////

type orbitPostNotificationResultRequest struct {
	OrbitNodeKey   string `json:"orbit_node_key"`
	NotificationID string `json:"notification_id"`
	Result         string `json:"result"`
	ExitCode       int    `json:"exit_code"`
}

func (r *orbitPostNotificationResultRequest) setOrbitNodeKey(nodeKey string) {
	r.OrbitNodeKey = nodeKey
}

func (r *orbitPostNotificationResultRequest) orbitHostNodeKey() string {
	return r.OrbitNodeKey
}

type orbitPostNotificationResultResponse struct {
	Err error `json:"error,omitempty"`
}

func (r orbitPostNotificationResultResponse) Error() error { return r.Err }

func postOrbitNotificationResultEndpoint(ctx context.Context, request interface{}, svc fleet.Service) (fleet.Errorer, error) {
	req := request.(*orbitPostNotificationResultRequest)
	if err := svc.SaveOrbitNotificationResult(ctx, req.NotificationID, req.Result, req.ExitCode); err != nil {
		return orbitPostNotificationResultResponse{Err: err}, nil
	}
	return orbitPostNotificationResultResponse{}, nil
}

func (svc *Service) SaveOrbitNotificationResult(ctx context.Context, notificationID string, result string, exitCode int) error {
	svc.authz.SkipAuthorization(ctx)

	host, ok := hostctx.FromContext(ctx)
	if !ok {
		return fleet.OrbitError{Message: "internal error: missing host from request context"}
	}

	return svc.ds.SetHostNotificationResult(ctx, &fleet.HostNotificationResultPayload{
		HostID:         host.ID,
		NotificationID: notificationID,
		Result:         result,
		ExitCode:       exitCode,
	})
}
