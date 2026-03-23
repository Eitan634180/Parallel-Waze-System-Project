package api

import "testing"

func TestParseSessionPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantID    string
		wantSub   string
		wantValid bool
	}{
		{name: "session root", path: "/session/abc", wantID: "abc", wantSub: "", wantValid: true},
		{name: "session websocket", path: "/session/abc/ws", wantID: "abc", wantSub: "ws", wantValid: true},
		{name: "missing id", path: "/session/", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotSub, gotValid := parseSessionPath(tt.path)
			if gotID != tt.wantID || gotSub != tt.wantSub || gotValid != tt.wantValid {
				t.Fatalf("parseSessionPath(%q) = (%q, %q, %t), want (%q, %q, %t)",
					tt.path, gotID, gotSub, gotValid, tt.wantID, tt.wantSub, tt.wantValid)
			}
		})
	}
}
