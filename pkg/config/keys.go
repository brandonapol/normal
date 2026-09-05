package config

var keyedCollections = map[string]string{
	"/spec/launcher/pages":                      "id",
	"/spec/launcher/pages/*/items":              "id",
	"/spec/apps/entries":                        "package",
	"/spec/notifications/quietHours":            "id",
	"/spec/notifications/rules":                 "id",
	"/spec/attention/infiniteScroll/detectors":  "id",
	"/spec/attention/infiniteScroll/exemptions": "id",
	"/spec/attention/sessionBudgets":            "id",
}

func KeyFieldFor(pattern string) (string, bool) {
	field, ok := keyedCollections[pattern]
	return field, ok
}

func KeyedCollections() map[string]string {
	out := make(map[string]string, len(keyedCollections))
	for pattern, field := range keyedCollections {
		out[pattern] = field
	}
	return out
}
