package audit

func (s *Service) RecordAuthzDenied(userID, role, requiredScope, method, path, reason, requestID string) {
	s.Record(RecordInput{
		UserID:    userID,
		Role:      role,
		Action:    "authorization",
		Method:    method,
		Path:      path,
		Result:    "denied",
		Reason:    requiredScope + ":" + reason,
		RequestID: requestID,
	})
}

func (s *Service) RecordAuthnFailure(method, path, reason, requestID string) {
	s.Record(RecordInput{
		Action:    "authentication",
		Method:    method,
		Path:      path,
		Result:    "failed",
		Reason:    reason,
		RequestID: requestID,
	})
}
