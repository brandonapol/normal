package config

const (
	APIVersion = "normal.os/v0"
	Kind       = "PhoneConfig"
)

type Config struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Spec       Spec     `json:"spec"`
}

type Metadata struct {
	Name        string            `json:"name"`
	Revision    int               `json:"revision"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type Spec struct {
	Launcher      Launcher      `json:"launcher"`
	Apps          Apps          `json:"apps"`
	Notifications Notifications `json:"notifications"`
	Attention     Attention     `json:"attention"`
}

type Launcher struct {
	Layout          string                   `json:"layout"`
	Columns         int                      `json:"columns"`
	MaxItemsPerPage int                      `json:"maxItemsPerPage"`
	Pages           []LauncherPage           `json:"pages"`
	Dock            []string                 `json:"dock"`
	AppDrawer       AppDrawer                `json:"appDrawer"`
	Gestures        map[string]GestureAction `json:"gestures"`
}

type LauncherPage struct {
	ID    string         `json:"id"`
	Title string         `json:"title,omitempty"`
	Items []LauncherItem `json:"items"`
}

type LauncherItem struct {
	Kind     string          `json:"kind"`
	ID       string          `json:"id"`
	Package  string          `json:"package,omitempty"`
	Label    string          `json:"label,omitempty"`
	Action   *ShortcutAction `json:"action,omitempty"`
	Provider string          `json:"provider,omitempty"`
	Size     string          `json:"size,omitempty"`
}

type ShortcutAction struct {
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
	URL     string `json:"url,omitempty"`
	Target  string `json:"target,omitempty"`
}

type AppDrawer struct {
	Enabled bool   `json:"enabled"`
	Sort    string `json:"sort"`
	Search  bool   `json:"search"`
}

type GestureAction struct {
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
	Target  string `json:"target,omitempty"`
}

type Apps struct {
	Policy  string     `json:"policy"`
	Entries []AppEntry `json:"entries"`
}

type AppEntry struct {
	Package        string                `json:"package"`
	Label          string                `json:"label,omitempty"`
	Source         string                `json:"source"`
	State          string                `json:"state"`
	Network        string                `json:"network"`
	SandboxProfile string                `json:"sandboxProfile,omitempty"`
	Permissions    map[string]string     `json:"permissions"`
	Attention      *AppAttentionOverride `json:"attention,omitempty"`
}

type AppAttentionOverride struct {
	Enforcement string `json:"enforcement,omitempty"`
	PageSize    int    `json:"pageSize,omitempty"`
}

type Notifications struct {
	DefaultDisposition string             `json:"defaultDisposition"`
	Bundling           Bundling           `json:"bundling"`
	QuietHours         []QuietHours       `json:"quietHours"`
	Rules              []NotificationRule `json:"rules"`
}

type Bundling struct {
	Enabled         bool     `json:"enabled"`
	DeliveryWindows []string `json:"deliveryWindows"`
}

type QuietHours struct {
	ID           string   `json:"id"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Days         []string `json:"days"`
	Breakthrough []string `json:"breakthrough"`
}

type NotificationRule struct {
	ID          string            `json:"id"`
	Priority    int               `json:"priority"`
	Match       NotificationMatch `json:"match"`
	Disposition string            `json:"disposition"`
}

type NotificationMatch struct {
	Package       string `json:"package,omitempty"`
	Channel       string `json:"channel,omitempty"`
	TitleContains string `json:"titleContains,omitempty"`
	Ongoing       *bool  `json:"ongoing,omitempty"`
}

func (m NotificationMatch) IsEmpty() bool {
	return m.Package == "" && m.Channel == "" && m.TitleContains == "" && m.Ongoing == nil
}

type Attention struct {
	InfiniteScroll InfiniteScrollPolicy `json:"infiniteScroll"`
	SessionBudgets []SessionBudget      `json:"sessionBudgets"`
}

type InfiniteScrollPolicy struct {
	Enforcement              string             `json:"enforcement"`
	PageSize                 int                `json:"pageSize"`
	MaxAutoLoads             int                `json:"maxAutoLoads"`
	Continuation             string             `json:"continuation"`
	ContinuationDelaySeconds int                `json:"continuationDelaySeconds"`
	Detectors                []Detector         `json:"detectors"`
	Exemptions               []Exemption        `json:"exemptions"`
	Webview                  WebViewEnforcement `json:"webview"`
}

type WebViewEnforcement struct {
	InjectShim                        bool    `json:"injectShim"`
	InterceptIntersectionObserver     bool    `json:"interceptIntersectionObserver"`
	InterceptHistoryScrollRestoration bool    `json:"interceptHistoryScrollRestoration"`
	MaxDocumentHeightMultiplier       float64 `json:"maxDocumentHeightMultiplier"`
}

type Detector struct {
	Kind          string   `json:"kind"`
	ID            string   `json:"id"`
	Signals       []string `json:"signals,omitempty"`
	MinConfidence float64  `json:"minConfidence,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	Package       string   `json:"package,omitempty"`
	Surface       string   `json:"surface,omitempty"`
}

type Exemption struct {
	ID        string `json:"id"`
	Package   string `json:"package"`
	Reason    string `json:"reason"`
	ExpiresAt string `json:"expiresAt"`
}

type SessionBudget struct {
	ID              string      `json:"id"`
	Scope           BudgetScope `json:"scope"`
	DailyMinutes    int         `json:"dailyMinutes"`
	SessionMinutes  int         `json:"sessionMinutes"`
	CooldownMinutes int         `json:"cooldownMinutes"`
	OnExhausted     string      `json:"onExhausted"`
}

type BudgetScope struct {
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
	Domain  string `json:"domain,omitempty"`
}
