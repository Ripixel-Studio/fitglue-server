package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	gatewaypb "github.com/fitglue/server/src/go/pkg/types/pb/gateway"
	pipelinemodelpb "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	usermodelpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

func (s *APIServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := 50
	if l := queryParam(r, "limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	pageToken := queryParam(r, "pageToken", "page_token")

	res, err := s.userService.ListUsers(ctx, &userpb.ListUsersRequest{
		Limit:     int32(limit),
		PageToken: pageToken,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	summaries := make([]*gatewaypb.AdminUserSummary, 0, len(res.GetUsers()))
	for _, p := range res.GetUsers() {
		summaries = append(summaries, s.userSummary(ctx, p))
	}

	WriteJSON(w, &gatewaypb.ListUsersAdminResponse{
		Users:         summaries,
		NextPageToken: res.GetNextPageToken(),
	})
}

func (s *APIServer) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}

	detail, err := s.buildUserDetail(r.Context(), userID)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, detail)
}

func (s *APIServer) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}

	var req struct {
		AccessEnabled *bool   `json:"accessEnabled"`
		Tier          *string `json:"tier"`
		IsAdmin       *bool   `json:"isAdmin"`
		DisplayName   *string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	// RBAC: an admin must not be able to lock themselves out of the console.
	if s.isSelf(r, userID) {
		if req.IsAdmin != nil && !*req.IsAdmin {
			WriteError(w, statusError(http.StatusBadRequest, "cannot remove your own admin role"))
			return
		}
		if req.AccessEnabled != nil && !*req.AccessEnabled {
			WriteError(w, statusError(http.StatusBadRequest, "cannot disable your own access"))
			return
		}
	}

	var tier *usermodelpb.UserTier
	if req.Tier != nil {
		v, ok := usermodelpb.UserTier_value[*req.Tier]
		if !ok {
			WriteError(w, statusError(http.StatusBadRequest, "invalid tier: "+*req.Tier))
			return
		}
		t := usermodelpb.UserTier(v)
		tier = &t
	}

	profile, err := s.userService.GetProfile(r.Context(), &userpb.GetProfileRequest{UserId: userID})
	if err != nil {
		WriteError(w, err)
		return
	}

	params := map[string]string{}
	if req.AccessEnabled != nil {
		profile.AccessEnabled = *req.AccessEnabled
		params["accessEnabled"] = strconv.FormatBool(*req.AccessEnabled)
	}
	if req.IsAdmin != nil {
		profile.IsAdmin = *req.IsAdmin
		params["isAdmin"] = strconv.FormatBool(*req.IsAdmin)
	}
	if req.DisplayName != nil {
		profile.DisplayName = *req.DisplayName
		params["displayName"] = *req.DisplayName
	}
	if tier != nil {
		profile.Tier = *tier
		params["tier"] = tier.String()
	}

	_, err = s.userService.UpdateProfile(r.Context(), &userpb.UpdateProfileRequest{
		UserId:  userID,
		Profile: profile,
	})
	s.auditAction(r.Context(), r, "update_user", userID, params, err)
	if err != nil {
		WriteError(w, err)
		return
	}

	detail, err := s.buildUserDetail(r.Context(), userID)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, detail)
}

// buildUserDetail assembles the aggregated 360-degree view of a single user.
// Every section beyond the core profile is best-effort: a failure in one
// dependent service degrades that section to empty rather than failing the
// whole request.
func (s *APIServer) buildUserDetail(ctx context.Context, userID string) (*gatewaypb.AdminUserDetail, error) {
	profile, err := s.userService.GetProfile(ctx, &userpb.GetProfileRequest{UserId: userID})
	if err != nil {
		return nil, err
	}
	detail := &gatewaypb.AdminUserDetail{Profile: profile}

	if integ, err := s.userService.ListIntegrations(ctx, &userpb.ListIntegrationsRequest{UserId: userID}); err == nil {
		detail.Integrations = integrationSummaries(integ)
	} else {
		s.logger.Warn(ctx, "admin: list integrations failed", "userId", userID, "error", err)
	}
	if pl, err := s.pipelineSvc.ListPipelines(ctx, &pipelinepb.ListPipelinesRequest{UserId: userID}); err == nil {
		detail.Pipelines = pipelineSummaries(pl.GetPipelines())
	}
	if s.activitySvc != nil {
		if stats, err := s.activitySvc.GetActivityStats(ctx, &activitypb.GetActivityStatsRequest{UserId: userID}); err == nil {
			detail.ActivityCount = stats.GetTotalActivities()
		}
	}
	if pending, err := s.pipelineSvc.ListPendingInputs(ctx, &pipelinepb.ListPendingInputsRequest{UserId: userID}); err == nil {
		detail.PendingInputs = pendingInputSummaries(pending.GetInputs())
	}
	if runs, err := s.pipelineSvc.AdminListPipelineRuns(ctx, &pipelinepb.AdminListPipelineRunsRequest{UserId: userID, Limit: 500}); err == nil {
		detail.PipelineRunCount = int32(len(runs.GetRuns()))
	}
	detail.Billing = s.userBilling(ctx, userID)
	return detail, nil
}

// userSummary maps a UserProfile into the directory row shape, enriching with
// per-user integration/pipeline counts (cheap at our user scale).
func (s *APIServer) userSummary(ctx context.Context, p *usermodelpb.UserProfile) *gatewaypb.AdminUserSummary {
	sum := &gatewaypb.AdminUserSummary{
		UserId:             p.GetUserId(),
		Email:              p.GetEmail(),
		DisplayName:        p.GetDisplayName(),
		CreatedAt:          p.GetCreatedAt(),
		Tier:               p.GetTier(),
		IsAdmin:            p.GetIsAdmin(),
		AccessEnabled:      p.GetAccessEnabled(),
		SyncCountThisMonth: p.GetSyncCountThisMonth(),
		PreventedSyncCount: p.GetPreventedSyncCount(),
		TrialEndsAt:        p.GetTrialEndsAt(),
	}
	if integ, err := s.userService.ListIntegrations(ctx, &userpb.ListIntegrationsRequest{UserId: p.GetUserId()}); err == nil {
		names := integrationProviderNames(integ)
		sum.Integrations = names
		sum.IntegrationCount = int32(len(names))
	}
	if pl, err := s.pipelineSvc.ListPipelines(ctx, &pipelinepb.ListPipelinesRequest{UserId: p.GetUserId()}); err == nil {
		sum.PipelineCount = int32(len(pl.GetPipelines()))
	}
	return sum
}

// integrationSummaries reflects over the UserIntegrations message and produces a
// redacted per-provider summary (status/health only — never tokens).
func integrationSummaries(ui *usermodelpb.UserIntegrations) []*gatewaypb.AdminIntegrationSummary {
	out := []*gatewaypb.AdminIntegrationSummary{}
	if ui == nil {
		return out
	}
	msg := ui.ProtoReflect()
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() || !msg.Has(fd) {
			continue
		}
		sub := msg.Get(fd).Message()
		summary := &gatewaypb.AdminIntegrationSummary{
			Provider:   string(fd.Name()),
			Connected:  true,
			Enabled:    subBool(sub, "enabled"),
			CreatedAt:  subTimestamp(sub, "created_at"),
			LastUsedAt: subTimestamp(sub, "last_used_at"),
			ExpiresAt:  subTimestamp(sub, "expires_at"),
		}
		summary.TokenHealth = tokenHealth(summary.ExpiresAt)
		out = append(out, summary)
	}
	return out
}

func integrationProviderNames(ui *usermodelpb.UserIntegrations) []string {
	names := []string{}
	for _, sum := range integrationSummaries(ui) {
		names = append(names, sum.GetProvider())
	}
	return names
}

func pipelineSummaries(cfgs []*pipelinemodelpb.PipelineConfig) []*gatewaypb.AdminPipelineSummary {
	out := []*gatewaypb.AdminPipelineSummary{}
	for _, c := range cfgs {
		source := c.GetSource()
		if source == "" && len(c.GetSources()) > 0 {
			source = c.GetSources()[0]
		}
		dests := []string{}
		for _, d := range c.GetDestinations() {
			dests = append(dests, d.String())
		}
		out = append(out, &gatewaypb.AdminPipelineSummary{
			Id:           c.GetId(),
			Name:         c.GetName(),
			Source:       source,
			Destinations: dests,
			Enabled:      !c.GetDisabled(),
		})
	}
	return out
}

func pendingInputSummaries(inputs []*pipelinemodelpb.PendingInput) []*gatewaypb.AdminPendingInputSummary {
	out := []*gatewaypb.AdminPendingInputSummary{}
	for _, in := range inputs {
		out = append(out, &gatewaypb.AdminPendingInputSummary{
			Id:                 in.GetActivityId(),
			ActivityId:         in.GetActivityId(),
			EnricherProviderId: in.GetEnricherProviderId(),
			Status:             in.GetStatus().String(),
			CreatedAt:          in.GetCreatedAt(),
		})
	}
	return out
}

func subBool(m protoreflect.Message, name string) bool {
	fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil || fd.Kind() != protoreflect.BoolKind {
		return false
	}
	return m.Get(fd).Bool()
}

func subTimestamp(m protoreflect.Message, name string) *timestamppb.Timestamp {
	fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil || fd.Kind() != protoreflect.MessageKind || !m.Has(fd) {
		return nil
	}
	if ts, ok := m.Get(fd).Message().Interface().(*timestamppb.Timestamp); ok {
		return ts
	}
	return nil
}

func tokenHealth(expires *timestamppb.Timestamp) string {
	if expires == nil {
		return "n/a"
	}
	if expires.AsTime().Before(time.Now()) {
		return "expired"
	}
	return "valid"
}
