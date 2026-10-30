package cloud

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

type MPATOSBucket struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

var tosBucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// ListMPATOSBuckets matches Studio's creation picker: current account/region,
// fixed agentkit-platform- prefix, and no project restriction or cloud writes.
func (c *Client) ListMPATOSBuckets(ctx context.Context, expected uint64, region string) ([]MPATOSBucket, *dto.SafeError) {
	if !ValidRegion(region) {
		return nil, dto.Failure("invalidInput", "platform", "never", 0)
	}
	credential, epoch, failure := c.snapshot()
	if failure != nil {
		return nil, failure
	}
	if epoch != expected || c.Epoch() != expected {
		return nil, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	if c.State().Principal == nil {
		return nil, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if !ValidProviderRegion(credential.Provider, region) {
		return nil, dto.Failure("invalidInput", "platform", "never", 0)
	}
	credentials := tos.NewStaticCredentials(credential.AccessKey, credential.SecretKey)
	credentials.WithSecurityToken(credential.Token)
	client, err := tos.NewClientV2("https://tos-"+region+"."+storageDomain(credential.Provider), tos.WithRegion(region), tos.WithCredentials(credentials), tos.WithMaxRetryCount(0), tos.WithHTTPTransport(c.transport))
	if err != nil {
		return nil, dto.Failure("connectionFailure", "platform", "readOnly", 0)
	}
	defer client.Close()
	result, err := client.ListBuckets(ctx, &tos.ListBucketsInput{})
	if c.Epoch() != expected {
		return nil, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	if err != nil {
		code, status := "connectionFailure", tos.StatusCode(err)
		if status == 401 {
			code = "authRequired"
		} else if status == 403 {
			code = "denied"
		}
		return nil, dto.Failure(code, "platform", "readOnly", status)
	}
	items := make([]MPATOSBucket, 0)
	seen := make(map[string]bool)
	for _, item := range result.Buckets {
		if (item.Location != "" && item.Location != region) || !strings.HasPrefix(item.Name, "agentkit-platform-") || !tosBucketName.MatchString(item.Name) || seen[item.Name] {
			continue
		}
		seen[item.Name] = true
		items = append(items, MPATOSBucket{Name: item.Name, Region: region})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}
