package perception

import (
	"testing"
)

func TestClassifyWorkApps(t *testing.T) {
	c := NewAppClassifier()
	tests := []struct {
		app  string
		want string // primary
		sub  string // subtype
		work bool
	}{
		{"Visual Studio Code", "work", "coding", true},
		{"Xcode", "work", "coding", true},
		{"IntelliJ IDEA", "work", "coding", true},
		{"GoLand", "work", "coding", true},
		{"Terminal", "work", "coding", true},
		{"iTerm2", "work", "coding", true},
		{"GitHub Desktop", "work", "coding", true},
		{"Slack", "work", "communication", true},
		{"Microsoft Teams", "work", "communication", true},
		{"Figma", "work", "productivity", true},
		{"Notion", "work", "productivity", true},
		{"Obsidian", "work", "productivity", true},
	}
	for _, tt := range tests {
		t.Run(tt.app, func(t *testing.T) {
			r := c.Classify(tt.app, "")
			if r.Primary != tt.want {
				t.Errorf("primary=%q want=%q", r.Primary, tt.want)
			}
			if r.Subtype != tt.sub {
				t.Errorf("subtype=%q want=%q", r.Subtype, tt.sub)
			}
			if r.IsWorking != tt.work {
				t.Errorf("isWorking=%v want=%v", r.IsWorking, tt.work)
			}
		})
	}
}

func TestClassifyPlayApps(t *testing.T) {
	c := NewAppClassifier()
	tests := []struct {
		app string
		sub string
	}{
		{"Steam", "gaming"},
		{"Spotify", "media"},
		{"Netflix", "media"},
	}
	for _, tt := range tests {
		t.Run(tt.app, func(t *testing.T) {
			r := c.Classify(tt.app, "")
			if r.Primary != "play" {
				t.Errorf("primary=%q want=play", r.Primary)
			}
			if r.Subtype != tt.sub {
				t.Errorf("subtype=%q want=%q", r.Subtype, tt.sub)
			}
		})
	}
}

func TestClassifySocialApps(t *testing.T) {
	c := NewAppClassifier()
	tests := []string{"微信", "WeChat", "QQ", "Telegram", "Discord"}
	for _, app := range tests {
		t.Run(app, func(t *testing.T) {
			r := c.Classify(app, "")
			if r.Primary != "social" {
				t.Errorf("%q: primary=%q want=social", app, r.Primary)
			}
		})
	}
}

func TestClassifyBrowserApps(t *testing.T) {
	c := NewAppClassifier()
	tests := []string{"Google Chrome", "Firefox", "Safari", "Arc", "Brave Browser"}
	for _, app := range tests {
		t.Run(app, func(t *testing.T) {
			r := c.Classify(app, "")
			if r.Primary != "idle" {
				t.Errorf("%q: primary=%q want=idle", app, r.Primary)
			}
			if r.Subtype != "browsing" {
				t.Errorf("%q: subtype=%q want=browsing", app, r.Subtype)
			}
		})
	}
}

func TestClassifyKeywordFallback(t *testing.T) {
	c := NewAppClassifier()
	// Unknown app but window title has Go code
	r := c.Classify("SomeApp", "main.go — sion-v1")
	if r.Primary != "work" || r.Subtype != "coding" {
		t.Errorf("got %s/%s, want work/coding", r.Primary, r.Subtype)
	}

	// Unknown app with game title
	r2 := c.Classify("UnknownApp", "League of Legends")
	if r2.Primary != "play" || r2.Subtype != "gaming" {
		t.Errorf("got %s/%s, want play/gaming", r2.Primary, r2.Subtype)
	}

	// Unknown app with chat title
	r3 := c.Classify("SomeApp", "聊天 - 工作群")
	if r3.Primary != "social" || r3.Subtype != "chatting" {
		t.Errorf("got %s/%s, want social/chatting", r3.Primary, r3.Subtype)
	}
}

func TestClassifyUnknown(t *testing.T) {
	c := NewAppClassifier()
	r := c.Classify("TotallyRandomApp", "some window")
	if r.Primary != "idle" {
		t.Errorf("primary=%q want=idle", r.Primary)
	}
	if r.Subtype != "unknown" {
		t.Errorf("subtype=%q want=unknown", r.Subtype)
	}
}

func TestClassifyMeetingApps(t *testing.T) {
	c := NewAppClassifier()
	tests := []string{"zoom.us", "腾讯会议", "FaceTime"}
	for _, app := range tests {
		t.Run(app, func(t *testing.T) {
			r := c.Classify(app, "")
			if r.Subtype != "meeting" {
				t.Errorf("%q: subtype=%q want=meeting", app, r.Subtype)
			}
		})
	}
}

func TestClassifyCaseInsensitive(t *testing.T) {
	c := NewAppClassifier()
	r1 := c.Classify("VISUAL STUDIO CODE", "")
	r2 := c.Classify("visual studio code", "")
	if r1.Primary != r2.Primary || r1.Subtype != r2.Subtype {
		t.Error("classification should be case-insensitive")
	}
}
