package domain

import "time"

const (
	SessionRoleOwner     = "owner"
	SessionRoleEditor    = "editor"
	SessionRoleViewer    = "viewer"
	SessionRoleModerator = "moderator"
)

type RoleAuditEntry struct {
	ActorID    string    `json:"actor_id"`
	TargetUser string    `json:"target_user"`
	OldRole    string    `json:"old_role"`
	NewRole    string    `json:"new_role"`
	ChangedAt  time.Time `json:"changed_at"`
}

type ChatSession struct {
	ID               string            `json:"id"`
	OwnerID          string            `json:"owner_id"`
	Participants     []string          `json:"participants"`
	ParticipantRoles map[string]string `json:"participant_roles"`
	RoleAudit        []RoleAuditEntry  `json:"role_audit,omitempty"`
	Messages         []Message         `json:"messages"`
	CreatedAt        time.Time         `json:"created_at"`
	IsActive         bool              `json:"is_active"`
}
