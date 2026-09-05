package engine

import (
	"sort"
	"strings"
)

const (
	ServiceLauncher    = "normal-launcher"
	ServiceAppd        = "normal-appd"
	ServiceNotifyd     = "normal-notifyd"
	ServiceAttentiond  = "normal-attentiond"
	ServiceWebviewShim = "normal-webview-shim"
)

type FileOwnership struct {
	File    string
	Sources []string
	Readers []string
}

func FileOwnerships() []FileOwnership {
	return []FileOwnership{
		{File: FileMetadata, Sources: []string{"/apiVersion", "/kind", "/metadata"}, Readers: nil},
		{File: FileLauncher, Sources: []string{"/spec/launcher"}, Readers: []string{ServiceLauncher}},
		{File: FileApps, Sources: []string{"/spec/apps"}, Readers: []string{ServiceAppd, ServiceLauncher}},
		{File: FileNotifications, Sources: []string{"/spec/notifications"}, Readers: []string{ServiceNotifyd}},
		{File: FileAttention, Sources: []string{"/spec/attention"}, Readers: []string{ServiceAttentiond}},
		{File: FileWebviewShim, Sources: []string{"/spec/attention", "/spec/apps"}, Readers: []string{ServiceWebviewShim}},
	}
}

func covers(source, path string) bool {
	return path == source || strings.HasPrefix(path, source+"/")
}

func FilesFor(path string) []string {
	files := make([]string, 0)
	for _, ownership := range FileOwnerships() {
		for _, source := range ownership.Sources {
			if covers(source, path) {
				files = append(files, ownership.File)
				break
			}
		}
	}
	return files
}

func IsOwnedPath(path string) bool {
	return len(FilesFor(path)) > 0
}

func ReadersOf(file string) []string {
	for _, ownership := range FileOwnerships() {
		if ownership.File == file {
			return ownership.Readers
		}
	}
	return nil
}

func ServicesForFiles(files []string) []string {
	seen := make(map[string]bool)
	services := make([]string, 0)
	for _, file := range files {
		for _, service := range ReadersOf(file) {
			if !seen[service] {
				seen[service] = true
				services = append(services, service)
			}
		}
	}
	sort.Strings(services)
	return services
}
