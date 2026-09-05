package normal

import (
	"list"
	"time"
	"strings"
)

limits: {
	minColumns:                   1
	maxColumns:                   8
	maxPages:                     8
	maxItemsPerPage:              24
	maxDockItems:                 5
	minPageSize:                  5
	maxPageSize:                  100
	maxAutoLoads:                 3
	maxContinuationDelaySeconds:  60
	maxDocumentHeightMultiplier:  4
	maxExemptions:                3
	maxExemptionDays:             30
	minExemptionReasonLength:     12
	maxNotificationRules:         64
	maxQuietHoursWindows:         8
	maxSessionBudgets:            32
	maxDetectors:                 32
	maxLabelCount:                16
}

#PackageID:  =~"^[a-zA-Z][a-zA-Z0-9_]*(\\.[a-zA-Z0-9_]+)+$"
#ResourceID: =~"^[a-z0-9][a-z0-9-]{0,63}$"
#TimeOfDay:  =~"^([01][0-9]|2[0-3]):[0-5][0-9]$"
#Timestamp:  time.Time
#Weekday:    "mon" | "tue" | "wed" | "thu" | "fri" | "sat" | "sun"

#PhoneConfig: {
	apiVersion: "normal.os/v0"
	kind:       "PhoneConfig"
	metadata:   #Metadata
	spec:       #Spec
}

#Metadata: {
	name:         string & !=""
	revision:     int & >=0
	description?: string
	labels?: {[string]: string}
}

#Spec: {
	launcher:      #Launcher
	apps:          #Apps
	notifications: #Notifications
	attention:     #Attention
}

#Launcher: {
	layout:          "grid" | "list"
	columns:         int & >=limits.minColumns & <=limits.maxColumns
	maxItemsPerPage: int & >=1 & <=limits.maxItemsPerPage
	pages: [...#LauncherPage] & list.MaxItems(limits.maxPages)
	dock: [...#PackageID] & list.MaxItems(limits.maxDockItems)
	appDrawer: #AppDrawer
	gestures: {[#Gesture]: #GestureAction}
}

#LauncherPage: {
	id:     #ResourceID
	title?: string
	items: [...#LauncherItem]
}

#LauncherItem: #AppItem | #ShortcutItem | #WidgetItem

#AppItem: {
	kind:    "app"
	id:      #ResourceID
	package: #PackageID
	label?:  string
}

#ShortcutItem: {
	kind:   "shortcut"
	id:     #ResourceID
	label:  string & !=""
	action: #ShortcutAction
}

#WidgetItem: {
	kind:     "widget"
	id:       #ResourceID
	provider: string & !=""
	size:     "1x1" | "2x1" | "2x2" | "4x1" | "4x2"
}

#ShortcutAction: {kind: "open-app", package: #PackageID} |
	{kind: "open-url", url: string & !=""} |
	{kind: "toggle", target: #ToggleTarget}

#ToggleTarget: "flashlight" | "do-not-disturb" | "wifi" | "airplane-mode" | "grayscale"

#AppDrawer: {
	enabled: bool
	sort:    "alphabetical" | "manual"
	search:  bool
}

#Gesture: "swipe-up" | "swipe-down" | "swipe-left" | "swipe-right" | "double-tap" | "long-press-home"

#GestureAction: {kind: "none"} |
	{kind: "open-drawer"} |
	{kind: "open-notifications"} |
	{kind: "open-app", package: #PackageID} |
	{kind: "toggle", target: #ToggleTarget}

#Apps: {
	policy: "allowlist" | "denylist"
	entries: [...#AppEntry]
}

#AppEntry: {
	package:         #PackageID
	label?:          string
	source:          "system" | "fdroid" | "play-compat" | "sideload"
	state:           "installed" | "blocked" | "absent"
	network:         "allow" | "wifi-only" | "deny"
	sandboxProfile?: string
	permissions: {[#Permission]: #PermissionDecision}
	attention?: #AppAttentionOverride
}

#Permission: "location" | "camera" | "microphone" | "contacts" |
	"storage" | "notifications" | "background-activity" | "nearby-devices"

#PermissionDecision: "allow" | "ask" | "deny"

#AppAttentionOverride: {
	enforcement?: #Enforcement
	pageSize?:    int & >=limits.minPageSize & <=limits.maxPageSize
}

#Notifications: {
	defaultDisposition: #Disposition
	bundling:           #Bundling
	quietHours: [...#QuietHours] & list.MaxItems(limits.maxQuietHoursWindows)
	rules: [...#NotificationRule] & list.MaxItems(limits.maxNotificationRules)
}

#Disposition: "allow" | "silence" | "bundle" | "block"

#Bundling: {
	enabled: bool
	deliveryWindows: [...#TimeOfDay]
}

#QuietHours: {
	id:    #ResourceID
	start: #TimeOfDay
	end:   #TimeOfDay
	days: [...#Weekday]
	breakthrough: [...#PackageID]
}

#NotificationRule: {
	id:          #ResourceID
	priority:    int & >=0 & <=1000
	match:       #NotificationMatch
	disposition: #Disposition
}

#NotificationMatch: {
	package?:       #PackageID
	channel?:       string
	titleContains?: string
	ongoing?:       bool
}

#Attention: {
	infiniteScroll: #InfiniteScrollPolicy
	sessionBudgets: [...#SessionBudget] & list.MaxItems(limits.maxSessionBudgets)
}

#Enforcement: "warn" | "paginate" | "block"

#ContinuationGate: "tap" | "tap-with-delay" | "hold" | "passphrase"

#InfiniteScrollPolicy: {
	enforcement:              #Enforcement
	pageSize:                 int & >=limits.minPageSize & <=limits.maxPageSize
	maxAutoLoads:             int & >=0 & <=limits.maxAutoLoads
	continuation:             #ContinuationGate
	continuationDelaySeconds: int & >=0 & <=limits.maxContinuationDelaySeconds

	detectors: [#Detector, ...#Detector] & list.MaxItems(limits.maxDetectors)

	exemptions: [...#Exemption] & list.MaxItems(limits.maxExemptions)

	webview: #WebViewEnforcement
}

#WebViewEnforcement: {
	injectShim:                        true
	interceptIntersectionObserver:     bool
	interceptHistoryScrollRestoration: bool
	maxDocumentHeightMultiplier:       number & >=1 & <=limits.maxDocumentHeightMultiplier
}

#Detector: #DomDetector | #AccessibilityDetector | #URLDetector | #SurfaceDetector

#DomDetector: {
	kind: "dom-heuristic"
	id:   #ResourceID
	signals: [#DomSignal, ...#DomSignal]
	minConfidence: number & >0 & <=1
}

#DomSignal: "append-on-scroll-mutation" | "sentinel-intersection-observer" |
	"unbounded-scroll-height" | "recycled-virtual-list" | "absent-pagination-controls"

#AccessibilityDetector: {
	kind: "accessibility-heuristic"
	id:   #ResourceID
	signals: [#AccessibilitySignal, ...#AccessibilitySignal]
	minConfidence: number & >0 & <=1
}

#AccessibilitySignal: "unbounded-collection-item-count" |
	"scroll-event-without-page-boundary" | "recycler-view-refill"

#URLDetector: {
	kind:    "url-pattern"
	id:      #ResourceID
	pattern: string & !=""
}

#SurfaceDetector: {
	kind:    "app-surface"
	id:      #ResourceID
	package: #PackageID
	surface: string & !=""
}

#Exemption: {
	id:        #ResourceID
	package:   #PackageID
	reason:    string & strings.MinRunes(limits.minExemptionReasonLength)
	expiresAt: #Timestamp
}

#SessionBudget: {
	id:              #ResourceID
	scope:           #BudgetScope
	dailyMinutes:    int & >=0 & <=1440
	sessionMinutes:  int & >=1 & <=1440
	cooldownMinutes: int & >=0 & <=1440
	onExhausted:     "warn" | "grayscale" | "lock"
}

#BudgetScope: {kind: "app", package: #PackageID} |
	{kind: "domain", domain: string & !=""} |
	{kind: "system-wide"}
