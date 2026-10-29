package usage

import (
	"net/url"
	"regexp"
	"strings"
)

const mainProfileMaskedHeader = "**********"

var mainProfileHeaderName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func normalizeMainProfileMCP(profile *MainProfile) {
	if len(profile.MCPSources) == 0 {
		profile.MCPSources = nil
	}
	if len(profile.MCPServers) == 0 {
		profile.MCPServers = nil
	}
	for name, server := range profile.MCPServers {
		if server.Headers == nil {
			server.Headers = map[string]string{}
		}
		profile.MCPServers[name] = server
	}
}
func mainProfileToolsValid(selection *[]string) bool {
	if selection == nil {
		return true
	}
	tools := *selection
	if len(tools) > 128 {
		return false
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if !mainProfileIdentifier(tool) || seen[tool] {
			return false
		}
		seen[tool] = true
	}
	return true
}
func mainProfileMCPValid(profile MainProfile) bool {
	if len(profile.MCPSources) > 32 || len(profile.MCPServers) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, source := range profile.MCPSources {
		if !mainProfileIdentifier(source.ResourceID) || strings.HasPrefix(source.ResourceID, "profile:") || seen[source.ResourceID] || !mainProfileToolsValid(source.EnabledTools) {
			return false
		}
		seen[source.ResourceID] = true
	}
	for name, server := range profile.MCPServers {
		u, err := url.Parse(server.URL)
		if !mainProfileSkillName.MatchString(name) || len(server.URL) > 2048 || err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(server.URL, " \t\n\r") || !mainProfileToolsValid(server.EnabledTools) || len(server.Headers) > 32 {
			return false
		}
		names := map[string]bool{}
		for key, value := range server.Headers {
			lower := strings.ToLower(key)
			if !mainProfileHeaderName.MatchString(key) || names[lower] || lower == "host" || lower == "x-ve-tip-token" || lower == "x-app-resourceid" || len(value) > 16384 || strings.ContainsAny(value, "\r\n") {
				return false
			}
			names[lower] = true
		}
	}
	return true
}
func mainProfileCredentialsReady(profile MainProfile) bool {
	for _, server := range profile.MCPServers {
		for _, value := range server.Headers {
			if len(value) > 8192 || value == mainProfileMaskedHeader || strings.HasPrefix(value, "fernet:v1:") {
				return false
			}
		}
	}
	return true
}
func publicMainProfile(profile MainProfile) MainProfile {
	record := cloneMainProfileRecord(MainProfileRecord{Profile: profile})
	for name, server := range record.Profile.MCPServers {
		for header := range server.Headers {
			server.Headers[header] = mainProfileMaskedHeader
		}
		record.Profile.MCPServers[name] = server
	}
	normalizeMainProfileMCP(&record.Profile)
	return record.Profile
}
