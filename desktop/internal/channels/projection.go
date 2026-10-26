package channels

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	qrcode "github.com/skip2/go-qrcode"
)

func enum(v string, values ...string) string {
	for _, value := range values {
		if v == value {
			return v
		}
	}
	return ""
}
func allowed(in []string, values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range in {
		if enum(v, values...) != "" && !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out
}
func token(v string, max int) string {
	if v == "" {
		return ""
	}
	if !validID(v, max) {
		return ""
	}
	return v
}
func plain(v string, max int) string {
	if utf8.RuneCountInString(v) > max || strings.ContainsAny(v, "\x00\r\n") {
		return ""
	}
	// Human-readable labels have no need to carry credential-bearing URLs.
	if strings.Contains(v, "://") || strings.Contains(v, "?token=") {
		return ""
	}
	return v
}
func digest(v string) string {
	if len(v) != 64 {
		return ""
	}
	for _, r := range v {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return ""
		}
	}
	return v
}
func timestamp(v string) string {
	if v == "" {
		return ""
	}
	if _, e := time.Parse(time.RFC3339Nano, v); e != nil {
		return ""
	}
	return v
}
func publicURL(v string) string {
	if v == "" || len(v) > 8192 {
		return ""
	}
	u, e := url.Parse(v)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.ContainsAny(v, "\r\n\x00") {
		return ""
	}
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}
func qrImage(v string) string {
	// QR authorization material is rendered only in the current operation. URL
	// queries and SVG/HTML are not forwarded to the webview.
	if len(v) > 2<<20 {
		return ""
	}
	for _, prefix := range []string{"data:image/png;base64,", "data:image/jpeg;base64,"} {
		if strings.HasPrefix(v, prefix) {
			raw, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, prefix))
			if e != nil || len(raw) == 0 {
				return ""
			}
			config, e := png.DecodeConfig(bytes.NewReader(raw))
			if e != nil || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
				return ""
			}
			return v
		}
	}
	return ""
}

func authorizedQR(loginURL, channel string) string {
	if loginURL == "" || len(loginURL) > 8192 || strings.ContainsAny(loginURL, "\r\n\x00") {
		return ""
	}
	u, e := url.Parse(loginURL)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Port() != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	allowedHost := false
	if channel == "feishu" {
		allowedHost = enum(host, "accounts.feishu.cn", "accounts.feishu-boe.cn", "accounts.feishu-pre.cn", "accounts.larksuite.com") != ""
	}
	if channel == "dingtalk" {
		allowedHost = host == "dingtalk.com" || strings.HasSuffix(host, ".dingtalk.com")
	}
	if !allowedHost {
		return ""
	}
	// The complete URL stays in Go memory. PNG is intentional one-operation
	// authorization material and must never be stored as ordinary account data.
	data, e := qrcode.Encode(loginURL, qrcode.Medium, 256)
	if e != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}
func cleanAccount(v Account) Account {
	v.AppName = plain(v.AppName, 256)
	v.BindingType = enum(v.BindingType, "shared", "private")
	if v.Channel != "feishu" {
		v.BindingType = ""
	}
	v.OwnerUserID = token(v.OwnerUserID, 256)
	v.BoundByUserID = token(v.BoundByUserID, 256)
	v.BoundByAuthMode = enum(v.BoundByAuthMode, "jwt", "runtime_key", "local")
	v.IMGatewayUUID = token(v.IMGatewayUUID, 256)
	v.GroupSessionScope = enum(v.GroupSessionScope, "group", "group_sender", "group_topic", "group_topic_sender")
	v.Config.Domain = enum(v.Config.Domain, "feishu", "lark")
	v.Config.OpenID = token(v.Config.OpenID, 256)
	v.Config.TenantBrand = plain(v.Config.TenantBrand, 128)
	v.Config.RobotName = plain(v.Config.RobotName, 256)
	v.CreatedAt = timestamp(v.CreatedAt)
	v.UpdatedAt = timestamp(v.UpdatedAt)
	return v
}
func cleanPermission(v Permission) Permission {
	v.ChatName = plain(v.ChatName, 256)
	v.Avatar = publicURL(v.Avatar)
	v.ChatMode = enum(v.ChatMode, "group", "topic")
	v.GroupSessionScope = enum(v.GroupSessionScope, "group", "group_sender", "group_topic", "group_topic_sender")
	v.CreatedAt = timestamp(v.CreatedAt)
	v.UpdatedAt = timestamp(v.UpdatedAt)
	return v
}

type bindingWire struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	AppID            string `json:"appId"`
	Domain           string `json:"domain"`
	BindingType      string `json:"bindingType"`
	QRCodeImage      string `json:"qrCodeImage"`
	LoginURL         string `json:"loginUrl"`
	ExpiresAt        string `json:"expiresAt"`
	PollAfterSeconds int    `json:"pollAfterSeconds"`
	LastErrorCode    string `json:"lastErrorCode"`
}

func projectBinding(v bindingWire, op, channel, domain, kind string) Binding {
	if v.Domain != "" {
		domain = enum(v.Domain, "feishu", "lark")
	}
	if v.BindingType != "" {
		kind = enum(v.BindingType, "shared", "private")
	}
	result := Binding{OperationID: op, ID: token(v.ID, 256), Channel: channel, AppID: token(v.AppID, 256), Status: enum(v.Status, "PENDING", "SCANNED", "COMPLETED", "AUTHORIZED", "REGISTERING", "BOUND", "FAILED", "EXPIRED"), QRCodeImage: qrImage(v.QRCodeImage), LoginURL: publicURL(v.LoginURL), ExpiresAt: timestamp(v.ExpiresAt), PollAfterSeconds: v.PollAfterSeconds, ErrorCode: enum(v.LastErrorCode, "REGISTRATION_UNCERTAIN", "CHANNEL_REGISTRATION_FAILED", "AUTHORIZATION_FAILED", "EXPIRED", "ACCESS_DENIED")}
	if channel == "feishu" {
		result.Domain = domain
		result.BindingType = kind
	}
	if result.PollAfterSeconds < 1 {
		result.PollAfterSeconds = 1
	}
	if result.PollAfterSeconds > 60 {
		result.PollAfterSeconds = 60
	}
	switch result.Status {
	case "PENDING", "SCANNED":
		result.State = "waiting"
	case "AUTHORIZED", "REGISTERING", "COMPLETED":
		result.State = "registering"
	case "BOUND":
		result.State = "registered"
	case "FAILED", "EXPIRED":
		result.State = "failed"
	default:
		result.State = "unknown"
	}
	if result.ErrorCode == "REGISTRATION_UNCERTAIN" {
		result.State = "unknown"
	}
	if result.State == "waiting" || result.State == "registering" {
		if v.LoginURL != "" {
			result.QRCodeImage = authorizedQR(v.LoginURL, channel)
		}
		if result.QRCodeImage == "" {
			result.LoginURL = ""
			if result.ErrorCode == "" {
				result.ErrorCode = "QR_MATERIAL_UNAVAILABLE"
			}
		}
	} else {
		result.QRCodeImage = ""
		result.LoginURL = ""
	}
	return result
}
func containsInputSecret(v interface{}, secrets ...string) bool {
	data, _ := json.Marshal(v)
	for _, secret := range secrets {
		escaped, _ := json.Marshal(secret)
		if secret != "" && strings.Contains(string(data), string(escaped[1:len(escaped)-1])) {
			return true
		}
	}
	return false
}
