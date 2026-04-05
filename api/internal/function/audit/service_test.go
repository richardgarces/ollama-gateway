package audit

import (
	"bytes"
	"strings"
	"testing"
)

func TestServiceRecordAndList(t *testing.T) {
	svc := NewService(3)
	svc.Record(RecordInput{UserID: "u1", Role: "admin", Action: "authorization", Method: "POST", Path: "/api/patch", Result: "denied", Reason: "patch:apply", RequestID: "r1"})
	svc.Record(RecordInput{UserID: "u2", Role: "viewer", Action: "authentication", Method: "GET", Path: "/api/security/scan/repo", Result: "failed", Reason: "invalid_token", RequestID: "r2"})

	items := svc.List(10, "", "")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].RequestID != "r2" {
		t.Fatalf("expected reverse chronological order")
	}
}

func TestWriteCSV(t *testing.T) {
	events := []Event{{Action: "authorization", Method: "POST", Path: "/api/patch", Result: "denied"}}
	buf := bytes.NewBuffer(nil)
	if err := WriteCSV(buf, events); err != nil {
		t.Fatalf("WriteCSV() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "timestamp,user_id,role,action,method,path,result,reason,request_id") {
		t.Fatalf("missing csv header")
	}
	if !strings.Contains(out, "authorization") {
		t.Fatalf("missing csv data row")
	}
}
