package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// WorkerMCPServers combines the deployed external MCP configuration with Runtime
// discovery metadata. Session overrides and Main Profile are deliberately not read.
func (s *Service) WorkerMCPServers(ctx context.Context, epoch uint64) (MCPDirectory, *dto.SafeError) {
	out := MCPDirectory{Servers: []MCPServer{}}
	add := func(server MCPServer) {
		for i, current := range out.Servers {
			if current.ID == server.ID || (current.Name == server.Name && current.Address == server.Address) {
				if server.ToolsKnown {
					out.Servers[i].Tools = server.Tools
					out.Servers[i].ToolsKnown = true
				}
				return
			}
		}
		out.Servers = append(out.Servers, server)
	}
	if s.cloud != nil {
		target, err := s.cloudTarget(epoch)
		if err != nil {
			return out, err
		}
		var runtime cloud.RuntimeRecord
		if err = s.cloudAction(ctx, epoch, "GetRuntime", map[string]string{"RuntimeId": target.RuntimeID}, &runtime, false); err != nil {
			return out, err
		}
		if runtime.RuntimeID != target.RuntimeID {
			return out, resourceResponseError(false)
		}
		env := map[string]string{}
		for _, item := range runtime.Envs {
			env[item.Key] = item.Value
		}
		servers, err := configuredMCP(env, "runtime")
		if err != nil {
			return out, err
		}
		for _, server := range servers {
			add(server)
		}
		if runtime.ToolID != "" {
			var tool struct {
				ToolID string `json:"ToolId"`
				Envs   []struct {
					Key   string `json:"Key"`
					Value string `json:"Value"`
				} `json:"Envs"`
			}
			if err = s.cloudAction(ctx, epoch, "GetTool", map[string]string{"ToolId": runtime.ToolID}, &tool, false); err != nil {
				return out, err
			}
			if tool.ToolID != runtime.ToolID {
				return out, resourceResponseError(false)
			}
			env = map[string]string{}
			for _, item := range tool.Envs {
				env[item.Key] = item.Value
			}
			servers, err = configuredMCP(env, "worker")
			if err != nil {
				return out, err
			}
			for _, server := range servers {
				add(server)
			}
		}
		if runtime.MCPToolsetID != "" {
			var toolset struct {
				Value *struct {
					ID       string `json:"MCPToolsetId"`
					Name     string `json:"Name"`
					Path     string `json:"Path"`
					Networks []struct {
						Type     string `json:"NetworkType"`
						Endpoint string `json:"Endpoint"`
					} `json:"NetworkConfigurations"`
				} `json:"MCPToolset"`
			}
			if err = s.cloudAction(ctx, epoch, "GetMCPToolset", map[string]string{"MCPToolsetId": runtime.MCPToolsetID}, &toolset, false); err != nil {
				return out, err
			}
			if toolset.Value == nil || toolset.Value.ID != runtime.MCPToolsetID {
				return out, resourceResponseError(false)
			}
			address := ""
			for _, network := range toolset.Value.Networks {
				if address == "" || network.Type == "public" {
					address = network.Endpoint
				}
				if network.Type == "public" {
					break
				}
			}
			address = strings.TrimRight(address, "/") + "/" + strings.TrimLeft(toolset.Value.Path, "/")
			if safeDisplayAddress(address) == "" {
				return out, resourceResponseError(false)
			}
			add(MCPServer{ID: runtime.MCPToolsetID, Name: toolset.Value.Name, Address: safeDisplayAddress(address), Source: "toolset"})
		}
	}
	var catalog struct {
		Resources []struct {
			ID          string    `json:"id"`
			Name        string    `json:"name"`
			Alias       string    `json:"alias"`
			Description string    `json:"description"`
			Tools       []MCPTool `json:"tools"`
		} `json:"resources"`
	}
	err := s.request(ctx, epoch, http.MethodGet, "/api/v1/debug/mcp-resources/current", nil, &catalog)
	if err != nil {
		// Older Runtime versions can still expose their deployed external configuration.
		if (err.HTTPStatus == 404 || err.HTTPStatus == 405) && s.cloud != nil {
			return out, nil
		}
		return out, resourceRequestError(err, false)
	}
	if catalog.Resources == nil {
		return out, resourceResponseError(false)
	}
	for _, item := range catalog.Resources {
		if !resourceIdentifierValid(item.ID) {
			return out, resourceResponseError(false)
		}
		merged := false
		for i, server := range out.Servers {
			if server.ID == item.ID || server.Name == item.Name {
				out.Servers[i].Tools = item.Tools
				out.Servers[i].ToolsKnown = item.Tools != nil
				merged = true
				break
			}
		}
		if !merged {
			add(MCPServer{ID: item.ID, Name: item.Name, Alias: item.Alias, Description: item.Description, Tools: item.Tools, ToolsKnown: item.Tools != nil, Source: "runtimeCatalog"})
		}
	}
	return out, s.resourceEpochError(epoch)
}

func safeDisplayAddress(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(raw, "\r\n\x00") {
		return ""
	}
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

func configuredMCP(env map[string]string, source string) ([]MCPServer, *dto.SafeError) {
	out := []MCPServer{}
	seen := map[string]bool{}
	appendServer := func(name, address, id string) bool {
		if name == "" {
			if u, e := url.Parse(address); e == nil {
				name = u.Hostname()
			}
			if name == "" {
				name = "mcp-" + strconv.Itoa(len(out)+1)
			}
		}
		if seen[name] || len(name) > 512 || safeDisplayAddress(address) == "" {
			return false
		}
		seen[name] = true
		if id == "" {
			id = source + ":" + name
		}
		out = append(out, MCPServer{ID: id, Name: name, Address: safeDisplayAddress(address), Source: source})
		return true
	}
	if raw := strings.TrimSpace(env["AGENTKIT_MCP_SERVERS"]); raw != "" {
		var config struct {
			Servers map[string]struct {
				URL string `json:"url"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal([]byte(raw), &config) != nil || config.Servers == nil || len(config.Servers) > 1000 {
			return nil, resourceResponseError(false)
		}
		// Sorted names keep the projection stable despite Go map iteration.
		names := make([]string, 0, len(config.Servers))
		for name := range config.Servers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			server := config.Servers[name]
			if !appendServer(name, server.URL, "agentkit:"+name) {
				return nil, resourceResponseError(false)
			}
		}
	}
	if raw := strings.TrimSpace(env["MCP_SERVERS_JSON"]); raw != "" {
		var servers []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if json.Unmarshal([]byte(raw), &servers) != nil || servers == nil || len(servers) > 32 {
			return nil, resourceResponseError(false)
		}
		for _, server := range servers {
			if !appendServer(server.Name, server.URL, "") {
				return nil, resourceResponseError(false)
			}
		}
	} else if raw := strings.TrimSpace(env["MCP_URLS"]); raw != "" {
		addresses := strings.Split(raw, ",")
		if len(addresses) > 32 {
			return nil, resourceResponseError(false)
		}
		for _, address := range addresses {
			address = strings.TrimSpace(address)
			if address != "" && !appendServer("", address, "") {
				return nil, resourceResponseError(false)
			}
		}
	}
	return out, nil
}
