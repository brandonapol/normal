package config

func systemApp(pkg, label, source string, permissions map[string]string) AppEntry {
	if permissions == nil {
		permissions = map[string]string{}
	}
	return AppEntry{
		Package:     pkg,
		Label:       label,
		Source:      source,
		State:       "installed",
		Network:     "allow",
		Permissions: permissions,
	}
}

func BaselineApps() []AppEntry {
	return []AppEntry{
		systemApp("os.normal.phone", "Phone", "system", map[string]string{
			"contacts": "allow", "notifications": "allow",
		}),
		systemApp("os.normal.messages", "Messages", "system", map[string]string{
			"contacts": "allow", "notifications": "allow",
		}),
		systemApp("os.normal.camera", "Camera", "system", map[string]string{
			"camera": "allow", "storage": "allow",
		}),
		systemApp("os.normal.browser", "Browser", "system", map[string]string{
			"location": "ask",
		}),
		systemApp("com.spotify.music", "Spotify", "play-compat", map[string]string{
			"notifications": "allow", "storage": "allow", "background-activity": "allow",
		}),
		systemApp("com.google.android.apps.maps", "Maps", "play-compat", map[string]string{
			"location": "ask", "notifications": "deny", "background-activity": "deny",
		}),
	}
}

func Baseline() Config {
	return Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:        "normal-baseline",
			Revision:    0,
			Description: "Default Normal phone configuration",
		},
		Spec: Spec{
			Launcher: Launcher{
				Layout:          "list",
				Columns:         1,
				MaxItemsPerPage: 8,
				Pages: []LauncherPage{{
					ID:    "home",
					Title: "Home",
					Items: []LauncherItem{
						{Kind: "app", ID: "home-phone", Package: "os.normal.phone"},
						{Kind: "app", ID: "home-messages", Package: "os.normal.messages"},
						{Kind: "app", ID: "home-maps", Package: "com.google.android.apps.maps"},
						{Kind: "app", ID: "home-spotify", Package: "com.spotify.music"},
					},
				}},
				Dock: []string{
					"os.normal.phone", "os.normal.messages",
					"os.normal.camera", "os.normal.browser",
				},
				AppDrawer: AppDrawer{Enabled: true, Sort: "alphabetical", Search: true},
				Gestures: map[string]GestureAction{
					"swipe-up":        {Kind: "open-drawer"},
					"swipe-down":      {Kind: "open-notifications"},
					"long-press-home": {Kind: "toggle", Target: "grayscale"},
				},
			},
			Apps: Apps{Policy: "allowlist", Entries: BaselineApps()},
			Notifications: Notifications{
				DefaultDisposition: "bundle",
				Bundling: Bundling{
					Enabled:         true,
					DeliveryWindows: []string{"09:00", "13:00", "18:00"},
				},
				QuietHours: []QuietHours{{
					ID:           "overnight",
					Start:        "22:00",
					End:          "07:00",
					Days:         []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
					Breakthrough: []string{"os.normal.phone"},
				}},
				Rules: []NotificationRule{
					{ID: "calls-always", Priority: 100, Disposition: "allow",
						Match: NotificationMatch{Package: "os.normal.phone"}},
					{ID: "messages-allow", Priority: 90, Disposition: "allow",
						Match: NotificationMatch{Package: "os.normal.messages"}},
					{ID: "no-ongoing-noise", Priority: 10, Disposition: "silence",
						Match: NotificationMatch{Ongoing: boolPtr(true)}},
				},
			},
			Attention: Attention{
				InfiniteScroll: InfiniteScrollPolicy{
					Enforcement:              "paginate",
					PageSize:                 20,
					MaxAutoLoads:             0,
					Continuation:             "tap-with-delay",
					ContinuationDelaySeconds: 3,
					Detectors: []Detector{
						{
							Kind: "dom-heuristic",
							ID:   "dom-default",
							Signals: []string{
								"append-on-scroll-mutation",
								"sentinel-intersection-observer",
								"unbounded-scroll-height",
								"absent-pagination-controls",
							},
							MinConfidence: 0.7,
						},
						{
							Kind: "accessibility-heuristic",
							ID:   "a11y-default",
							Signals: []string{
								"unbounded-collection-item-count",
								"recycler-view-refill",
							},
							MinConfidence: 0.75,
						},
					},
					Exemptions: []Exemption{},
					Webview: WebViewEnforcement{
						InjectShim:                        true,
						InterceptIntersectionObserver:     true,
						InterceptHistoryScrollRestoration: true,
						MaxDocumentHeightMultiplier:       2,
					},
				},
				SessionBudgets: []SessionBudget{{
					ID:              "browser-daily",
					Scope:           BudgetScope{Kind: "app", Package: "os.normal.browser"},
					DailyMinutes:    60,
					SessionMinutes:  15,
					CooldownMinutes: 30,
					OnExhausted:     "grayscale",
				}},
			},
		},
	}
}

func boolPtr(v bool) *bool { return &v }
