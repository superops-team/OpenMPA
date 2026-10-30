package cloud

import "github.com/superops-team/OpenMPA/desktop/internal/dto"

// Empty provider is the legacy domestic account. Never infer it from an endpoint.
func NormalizeProvider(provider string) string {
	if provider == "" {
		return "volcengine"
	}
	return provider
}
func ValidProviderRegion(provider, region string) bool {
	switch NormalizeProvider(provider) {
	case "volcengine":
		return region == "cn-beijing" || region == "cn-shanghai"
	case "byteplus":
		return region == "ap-southeast-1"
	}
	return false
}
func providerScopeAccount(provider, account string) string {
	if NormalizeProvider(provider) == "byteplus" {
		return "byteplus:" + account
	}
	return account
}

// AllRegions is the login catalog; callers filter it by the selected provider.
func AllRegions() []dto.RegionSummary {
	items := Regions()
	for i := range items {
		items[i].Provider = "volcengine"
	}
	return append(items, dto.RegionSummary{Provider: "byteplus", Region: "ap-southeast-1", DisplayName: "Singapore", EndpointHost: "agentkit.ap-southeast-1.byteplusapi.com"})
}
func providerEndpoint(provider, service, region string) string {
	if NormalizeProvider(provider) == "byteplus" {
		switch service {
		case "sts", "ark":
			return "https://open.byteplusapi.com"
		case "vefaas", "agentkit":
			return "https://" + service + "." + region + ".byteplusapi.com"
		}
		return ""
	}
	switch service {
	case "sts":
		return "https://sts.volcengineapi.com"
	case "ark", "vefaas":
		return "https://open.volcengineapi.com"
	case "agentkit":
		endpoint, _ := AgentKitEndpoint(region)
		return endpoint
	}
	return ""
}
func modelRegion(provider string) string {
	if NormalizeProvider(provider) == "byteplus" {
		return "ap-southeast-1"
	}
	return "cn-beijing"
}
func modelBase(provider string) string {
	if NormalizeProvider(provider) == "byteplus" {
		return "https://ark.ap-southeast.bytepluses.com/api/v3"
	}
	return "https://ark.cn-beijing.volces.com/api/v3"
}
func storageDomain(provider string) string {
	if NormalizeProvider(provider) == "byteplus" {
		return "bytepluses.com"
	}
	return "volces.com"
}
