package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/timestamppb"

	gatewaypb "github.com/fitglue/server/src/go/pkg/types/pb/gateway"
)

const auditCollection = "admin_audit_log"

// adminActor extracts the authenticated admin's uid and email from the request
// context (populated by AdminMiddleware after verifying the Firebase token).
func adminActor(r *http.Request) (uid, email string) {
	token, ok := r.Context().Value(userContextKey).(*auth.Token)
	if !ok || token == nil {
		return "", ""
	}
	uid = token.UID
	if e, ok := token.Claims["email"].(string); ok {
		email = e
	}
	return uid, email
}

// auditAction records an admin mutation to the admin_audit_log collection. It is
// best-effort: a failure to persist the audit entry is logged but never fails
// the underlying operation. opErr captures whether the audited operation itself
// succeeded so the log reflects attempts as well as successes.
func (s *APIServer) auditAction(ctx context.Context, r *http.Request, action, targetUserID string, params map[string]string, opErr error) {
	if s.firestoreClient == nil {
		return // best-effort; nothing to write to (e.g. in unit tests)
	}
	actorUID, actorEmail := adminActor(r)
	result := "ok"
	errMsg := ""
	if opErr != nil {
		result = "error"
		errMsg = opErr.Error()
	}
	if params == nil {
		params = map[string]string{}
	}
	entry := map[string]interface{}{
		"actor_uid":      actorUID,
		"actor_email":    actorEmail,
		"action":         action,
		"target_user_id": targetUserID,
		"params":         params,
		"result":         result,
		"error":          errMsg,
		"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if _, _, err := s.firestoreClient.Collection(auditCollection).Add(ctx, entry); err != nil {
		s.logger.Warn(ctx, "failed to write admin audit log", "action", action, "error", err)
	}
}

func (s *APIServer) handleListAuditLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	target := queryParam(r, "targetUserId", "target_user_id")

	limit := 100
	if l := queryParam(r, "limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	q := s.firestoreClient.Collection(auditCollection).OrderBy("timestamp", firestore.Desc)
	if target != "" {
		q = q.Where("target_user_id", "==", target)
	}

	iter := q.Limit(limit).Documents(ctx)
	defer iter.Stop()

	entries := make([]*gatewaypb.AdminAuditLogEntry, 0, limit)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.logger.Error(ctx, "failed to list audit log", "error", err)
			WriteError(w, err)
			return
		}
		entries = append(entries, auditEntryFromDoc(doc.Ref.ID, doc.Data()))
	}

	WriteJSON(w, &gatewaypb.ListAuditLogAdminResponse{Entries: entries})
}

func auditEntryFromDoc(id string, data map[string]interface{}) *gatewaypb.AdminAuditLogEntry {
	entry := &gatewaypb.AdminAuditLogEntry{
		Id:           id,
		ActorUid:     asString(data["actor_uid"]),
		ActorEmail:   asString(data["actor_email"]),
		Action:       asString(data["action"]),
		TargetUserId: asString(data["target_user_id"]),
		Result:       asString(data["result"]),
		Error:        asString(data["error"]),
		Params:       map[string]string{},
	}
	if params, ok := data["params"].(map[string]interface{}); ok {
		for k, v := range params {
			entry.Params[k] = asString(v)
		}
	}
	if ts, err := time.Parse(time.RFC3339Nano, asString(data["timestamp"])); err == nil {
		entry.Timestamp = timestamppb.New(ts)
	}
	return entry
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}
