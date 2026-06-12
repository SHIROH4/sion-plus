package perception

import (
	"strings"

	"github.com/shirohania/sion/internal/port"
)

// AppClassifier implements port.AppClassifier.
// ~80-app hardcoded mapping with window-title keyword fallback.
type AppClassifier struct{}

var _ port.AppClassifier = (*AppClassifier)(nil)

func NewAppClassifier() *AppClassifier { return &AppClassifier{} }

func (c *AppClassifier) Classify(appName, windowTitle string) *port.AppClassification {
	name := strings.ToLower(appName)
	title := strings.ToLower(windowTitle)

	// Meeting check FIRST — before work/communication catches "zoom"/"teams"
	if cls := matchMeeting(name); cls != nil {
		return cls
	}
	if cls := matchExact(name); cls != nil {
		return cls
	}
	if cls := matchByKeyword(name, title); cls != nil {
		return cls
	}
	return &port.AppClassification{Primary: "idle", Subtype: "unknown"}
}

// ── Meeting (checked first to avoid work/comm false positives) ─────

func matchMeeting(name string) *port.AppClassification {
	if contains(name, "zoom.us") || contains(name, "腾讯会议") || contains(name, "voov") ||
		contains(name, "webex") || contains(name, "facetime") || contains(name, "google meet") {
		return &port.AppClassification{Primary: "social", Subtype: "meeting", IsWorking: true}
	}
	return nil
}

// ── Exact matches ──────────────────────────────────────────────────

func matchExact(name string) *port.AppClassification {
	switch {
	// ── Work / Coding ──
	case contains(name, "visual studio"), contains(name, "vscode"),
		contains(name, "xcode"), contains(name, "intellij"), contains(name, "android studio"),
		contains(name, "pycharm"), contains(name, "goland"), contains(name, "webstorm"),
		contains(name, "eclipse"), contains(name, "sublime"), contains(name, "atom"),
		contains(name, "zed"), contains(name, "cursor"), contains(name, "fleet"),
		exactName(name, "code"): // "code" alone is VS Code, not a substring trap
		return &port.AppClassification{Primary: "work", Subtype: "coding", IsWorking: true}

	case contains(name, "terminal"), contains(name, "iterm"), contains(name, "warp"),
		contains(name, "kitty"), contains(name, "alacritty"), contains(name, "hyper"):
		return &port.AppClassification{Primary: "work", Subtype: "coding", IsWorking: true}

	case contains(name, "sourcetree"), contains(name, "github desktop"),
		contains(name, "tower"), exactMatch(name, "git", "fork"):
		return &port.AppClassification{Primary: "work", Subtype: "coding", IsWorking: true}

	// ── Work / Communication ──
	case contains(name, "slack"), contains(name, "microsoft teams"),
		contains(name, "outlook"), contains(name, "thunderbird"),
		contains(name, "spark"), contains(name, "mimestream"),
		exactMatch(name, "mail", "teams"):
		return &port.AppClassification{Primary: "work", Subtype: "communication", IsWorking: true}

	// ── Work / Design & Docs ──
	case contains(name, "figma"), contains(name, "sketch"), contains(name, "photoshop"),
		contains(name, "illustrator"), contains(name, "blender"), contains(name, "autocad"),
		contains(name, "notion"), contains(name, "obsidian"), contains(name, "logseq"),
		contains(name, "word"), contains(name, "excel"), contains(name, "powerpoint"),
		contains(name, "pages"), contains(name, "numbers"), contains(name, "keynote"):
		return &port.AppClassification{Primary: "work", Subtype: "productivity", IsWorking: true}

	// ── Social / Chatting ──
	case contains(name, "微信"), contains(name, "wechat"), contains(name, "qq"),
		contains(name, "telegram"), contains(name, "whatsapp"), contains(name, "signal"),
		contains(name, "line"), contains(name, "messenger"), contains(name, "imessage"),
		contains(name, "discord"):
		return &port.AppClassification{Primary: "social", Subtype: "chatting", IsWorking: false}

	// ── Social / Browsing ──
	case contains(name, "微博"), contains(name, "twitter"), exactName(name, "x"),
		contains(name, "instagram"), contains(name, "threads"), contains(name, "mastodon"):
		return &port.AppClassification{Primary: "social", Subtype: "browsing", IsWorking: false}

	// ── Play / Gaming ──
	case contains(name, "steam"), contains(name, "epic games"), contains(name, "battle.net"),
		contains(name, "riot"), contains(name, "ubisoft"), contains(name, "gog"):
		return &port.AppClassification{Primary: "play", Subtype: "gaming", IsWorking: false}

	// ── Browser ──
	case contains(name, "chrome"), contains(name, "firefox"), contains(name, "safari"),
		contains(name, "edge"), contains(name, "brave"), contains(name, "arc"),
		contains(name, "opera"), contains(name, "vivaldi"):
		return &port.AppClassification{Primary: "idle", Subtype: "browsing", IsWorking: false}

	// ── Media / Entertainment ──
	case contains(name, "spotify"), contains(name, "apple music"), contains(name, "music"),
		contains(name, "netflix"), contains(name, "youtube"), contains(name, "bilibili"),
		contains(name, "哔哩哔哩"), contains(name, "iina"), contains(name, "vlc"),
		contains(name, "quicktime"), contains(name, "preview"), contains(name, "photos"):
		return &port.AppClassification{Primary: "play", Subtype: "media", IsWorking: false}


		// ── Self (safety net — Sion's own process) ──
		case contains(name, "electron"), contains(name, "sion"):
			return &port.AppClassification{Primary: "idle", Subtype: "self", IsWorking: false}

	// ── System ──
	case contains(name, "finder"), contains(name, "访达"), contains(name, "settings"),
		contains(name, "系统设置"), contains(name, "preferences"), contains(name, "activity monitor"):
		return &port.AppClassification{Primary: "idle", Subtype: "system", IsWorking: false}

	default:
		return nil
	}
}

// ── Keyword fallback ───────────────────────────────────────────────

func matchByKeyword(_, title string) *port.AppClassification {
	gameWords := []string{"game", "游戏", "league of legends", "minecraft", "genshin",
		"原神", "valorant", "dota", "cs2", "counter-strike", "fortnite", "elden ring",
		"baldur", "wow", "world of warcraft", "星穹铁道", "star rail"}
	for _, w := range gameWords {
		if strings.Contains(title, w) {
			return &port.AppClassification{Primary: "play", Subtype: "gaming", IsWorking: false}
		}
	}

	workWords := []string{".go", ".py", ".rs", ".tsx", ".ts", ".js", ".java", ".kt",
		".swift", ".c ", ".cpp", ".h ", "docker", "kubernetes", ".yaml", ".yml",
		".toml", "config", "deploy", "pipeline", "github", "gitlab", "jira", "linear", "figma"}
	for _, w := range workWords {
		if strings.Contains(title, w) {
			return &port.AppClassification{Primary: "work", Subtype: "coding", IsWorking: true}
		}
	}

	socialWords := []string{"聊天", "chat", "message", "DM", "私信", "群聊"}
	for _, w := range socialWords {
		if strings.Contains(title, w) {
			return &port.AppClassification{Primary: "social", Subtype: "chatting", IsWorking: false}
		}
	}

	return nil
}

// ── Helpers ────────────────────────────────────────────────────────

func contains(s, substr string) bool { return strings.Contains(s, substr) }

// exactName matches only if s equals one of the given candidates.
func exactName(s string, candidates ...string) bool {
	for _, c := range candidates {
		if s == c {
			return true
		}
	}
	return false
}

// exactMatch matches if s equals or contains any candidate as a standalone token.
func exactMatch(s string, candidates ...string) bool {
	for _, c := range candidates {
		if s == c || strings.HasPrefix(s, c+" ") || strings.HasSuffix(s, " "+c) ||
			strings.Contains(s, " "+c+" ") {
			return true
		}
	}
	return false
}
