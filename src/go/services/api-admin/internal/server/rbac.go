package server

import "net/http"

// isSelf reports whether the authenticated admin is acting on their own account.
// Used to block actions that would lock an admin out of the console (removing
// their own admin role, disabling their own access, or deleting themselves).
func (s *APIServer) isSelf(r *http.Request, targetUserID string) bool {
	uid, _ := adminActor(r)
	return uid != "" && uid == targetUserID
}
