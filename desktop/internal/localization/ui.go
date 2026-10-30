package localization

import "strings"

var uiKeys = map[string]string{
	"Settings": "settings", "Channels": "channels", "Scheduled tasks": "scheduledTasks", "Knowledge": "knowledge", "Working…": "working",
	"Resources": "resources", "Collaboration": "collaboration",
	"Skills, MCP & instructions": "skillsMcpInstructions", "A2A collaboration": "a2aCollaboration",
	"Account": "account", "Runtime": "runtime", "Authentication": "authentication", "Deployment": "deployment", "Language": "language",
	"Current Runtime": "currentRuntime", "Runtime for this account": "runtimeForThisAccount", "Default Runtime": "defaultRuntime", "Last selected Runtime": "lastSelectedRuntime", "Last Runtime connection restored.": "lastRuntimeRestored",
	"Select Runtime": "selectRuntime",
	"Switch account": "switchAccount", "Checking availability in the current account…": "checkingRuntimeAvailability",
	"The directory loads automatically. Select a Runtime to switch immediately.": "runtimeSwitchDescription",
	"No MPA Runtimes are visible in this account, Region and Project.":           "noVisibleRuntimes",
	"Display language": "displayLanguage", "Follow system": "followSystem", "中文": "chinese", "English": "english",
	"OpenMPA uses one language across the desktop UI, system menus and content pages.": "languageDescription", "Display language updated.": "languageChanged",
	"Volcengine account": "volcengineAccount", "Sign out": "signOut", "Access key": "accessKey", "Secret key": "secretKey",
	"Security token (optional)": "securityTokenOptional", "Login region": "loginRegion", "Sign in": "signIn",
	"Credentials remain in this OpenMPA process and are cleared on exit.": "credentialsSessionOnly", "Choose an MPA Runtime": "chooseRuntime", "Region": "region", "Project": "project",
	"Refresh MPA Runtimes": "refreshRuntimes", "Find by name or ID": "findRuntime", "Connect selected Runtime": "connectRuntime", "Switch Runtime": "switchRuntime",
	"Runtime connected. You can start conversations now; an explicit business identity is optional.": "runtimeConnected",
	"Session identity: Runtime key default · optional override in Authentication":                    "runtimeKeyIdentity", "Session identity: explicit business identity": "explicitIdentity",
	"Business identity": "businessIdentity", "Business user ID": "businessUserId", "Legacy JWT": "legacyJwt",
	"Verified test deployment with JWT disabled": "verifiedTest", "Save business identity": "saveBusinessIdentity", "Test connection": "testConnection",
	"Channel administrator": "channelAdministrator", "Save channel authentication": "saveChannelAuthentication", "Choose and connect a Runtime first.": "chooseRuntimeFirst",
	"Runtime resources": "runtimeResources", "Read current resources": "readCurrentResources", "Runtime and Worker images": "runtimeImages", "Read current images": "readCurrentImages",
	"Channel authentication mode": "channelAuthenticationMode", "Channel credential": "channelCredential",
	"Description": "description", "CPU (milli cores)": "cpuMilli", "Memory (MB)": "memoryMb", "Minimum instances": "minimumInstances",
	"Maximum instances": "maximumInstances", "Maximum concurrency": "maximumConcurrency", "Update Runtime resources": "updateRuntimeResources",
	"Resource update operation ID": "resourceOperationId", "Recover resource update receipt": "recoverResourceReceipt", "Read resource update receipt": "readResourceReceipt",
	"Runtime and Worker Tool images": "runtimeWorkerImages", "Runtime image": "runtimeImage", "Update Runtime image": "updateRuntimeImage",
	"Worker image": "workerImage", "Update Worker Tool image": "updateWorkerImage", "Refresh saved image receipts": "refreshImageReceipts",
	"Business identity saved.": "configurationSaved", "Channel authentication saved.": "channelAuthenticationSaved",
	"Saved credentials": "savedCredentials", "Passphrase": "passphrase", "Confirm passphrase": "confirmPassphrase",
	"Show passphrase": "showPassphrase", "Unlock and sign in": "unlockAndSignIn",
	"Forget saved credentials": "forgetSavedCredentials", "Save credentials": "saveCredentials",
	"Delete the encrypted credential from this Mac? You will need to enter AK/SK again.": "forgetVaultConfirm",
	"New assistant": "newAssistant", "Assistant name": "assistantName", "Edit": "editAssistant", "Delete": "delete", "New Space": "newSpace", "Space name": "spaceName",
	"Rename conversation": "renameConversation", "Conversation title": "conversationTitle", "Cancel": "cancel", "Create": "create", "Save title": "saveTitle",
	"Finish the current operation before creating another item.": "finishCurrentOperation", "OpenMPA home": "openMpaHome", "Toggle sidebar": "toggleSidebar",
	"Toggle navigation": "toggleNavigation", "New conversation": "newConversation", "Open Spaces": "openSpaces", "Open scheduled activity": "openScheduledActivity",
	"Open settings": "openSettings", "Show capabilities and artifacts": "showCapabilities", "OpenMPA title bar": "titleBar", "Title bar separator": "titleBarSeparator",
	"Current location": "currentLocation", "Your workspace": "yourWorkspace", "chat": "chat", "Workspace navigation": "workspaceNavigation",
	"Find conversations": "findConversations", "Find assistants": "findConversations", "Search conversations": "searchConversations", "Search assistants": "searchConversations", "ASSISTANTS": "assistants", "SPACES": "spaces",
	"CONVERSATIONS": "conversations", "Configured skills only. Browsing the AgentKit skill space needs a catalog API this client does not have.": "assistantSkillScope", "No conversations yet.": "noConversations", "No assistants yet.": "noConversations", "Assistant": "assistant", "Select an assistant": "selectAssistant", "Home": "home",
	"SETTINGS": "settingsSectionsTitle", "Theme": "theme", "Session knowledge": "sessionKnowledge",
	"Select a conversation first.": "selectConversationFirst", "Scheduled tasks for this assistant.": "scheduledForAssistant",
	"Select a conversation to scope scheduled tasks to its assistant.": "selectConversationForSchedules",
	"Display theme": "displayTheme", "The theme applies to the desktop shell and the conversation content.": "themeDescription",
	"Light": "light", "Dark": "dark", "Display theme updated.": "themeChanged",
	"RECENT CHATS": "recentChats", "Session": "session", "Choose your Runtime in Settings.": "chooseRuntimeSettings", "Scheduled & activity": "scheduledActivity",
	"Channels & templates": "channelsTemplates", "Settings & setup": "settingsSetup", "Host operation status": "nativeOperationStatus", "Dismiss": "dismiss",
	"Runtime key identity": "runtimeKeyStatus", "Explicit business identity": "explicitIdentityStatus", "Authentication required": "authenticationRequired",
}

// UI translates a fixed product string. Dynamic user and resource values fall
// through byte-for-byte; a leading selection mark does not change its key.
func UI(value string) string {
	prefix := ""
	plain := value
	if strings.HasPrefix(plain, "✓ ") {
		prefix, plain = "✓ ", strings.TrimPrefix(plain, "✓ ")
	}
	if key := uiKeys[plain]; key != "" {
		return prefix + Text(key, nil)
	}
	if strings.HasPrefix(plain, "Recover the original assistant: ") {
		return Text("recoverAssistant", map[string]any{"session": strings.TrimPrefix(plain, "Recover the original assistant: ")})
	}
	if strings.HasPrefix(plain, "Knowledge for conversation ") {
		return Text("knowledgeForConversation", map[string]any{"session": strings.TrimPrefix(plain, "Knowledge for conversation ")})
	}
	return value
}

func Error(code, fallback string) string {
	if key := map[string]string{"notConnected": "notConnected", "authRequired": "authRequired", "invalidInput": "invalidInput"}[code]; key != "" {
		return Text(key, nil)
	}
	return fallback
}
