package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

type SkillArchiveObject struct{ URL, Bucket string }

// PutSkillArchive stores a content-addressed private ZIP with host credentials.
// The caller cannot supply an endpoint, bucket, object path or public ACL.
func (c *Client) PutSkillArchive(ctx context.Context, region, project string, data []byte) (SkillArchiveObject, *dto.SafeError) {
	empty := SkillArchiveObject{}
	if !ValidRegion(region) || len(data) == 0 || len(data) > 20<<20 {
		return empty, dto.Failure("invalidInput", "platform", "never", 0)
	}
	cred, epoch, err := c.snapshot()
	if err != nil {
		return empty, err
	}
	state := c.State()
	if state.Epoch != epoch {
		return empty, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	if state.Principal == nil {
		return empty, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	owner := sha256.Sum256([]byte(state.Principal.AccountID + "/" + project))
	bucket := fmt.Sprintf("openmpa-skills-%x-%s", owner[:8], region)
	digest := sha256.Sum256(data)
	key := fmt.Sprintf("agentkit/skills/%x.zip", digest)
	if !ValidProviderRegion(cred.Provider, region) {
		return empty, dto.Failure("invalidInput", "platform", "never", 0)
	}
	credentials := tos.NewStaticCredentials(cred.AccessKey, cred.SecretKey)
	credentials.WithSecurityToken(cred.Token)
	client, e := tos.NewClientV2("https://tos-"+region+"."+storageDomain(cred.Provider), tos.WithRegion(region), tos.WithCredentials(credentials), tos.WithMaxRetryCount(0), tos.WithHTTPTransport(c.transport))
	if e != nil {
		return empty, storageFailure(e)
	}
	defer client.Close()
	buckets, e := client.ListBuckets(ctx, &tos.ListBucketsInput{ProjectName: project})
	if e != nil {
		return empty, storageFailure(e)
	}
	owned := false
	for _, item := range buckets.Buckets {
		if item.Name == bucket {
			if item.Location != region {
				return empty, dto.Failure("scopeMismatch", "platform", "never", 0)
			}
			owned = true
			break
		}
	}
	if !owned {
		if c.Epoch() != epoch {
			return empty, dto.Failure("staleConnection", "platform", "reload", 0)
		}
		if _, e = client.CreateBucketV2(ctx, &tos.CreateBucketV2Input{Bucket: bucket, ProjectName: project}); e != nil {
			return empty, storageFailure(e)
		}
		buckets, e = client.ListBuckets(ctx, &tos.ListBucketsInput{ProjectName: project})
		if e != nil {
			return empty, storageFailure(e)
		}
		for _, item := range buckets.Buckets {
			if item.Name == bucket && item.Location == region {
				owned = true
				break
			}
		}
		if !owned {
			return empty, dto.Failure("skillStorageFailed", "platform", "readOnly", 0)
		}
	}

	if c.Epoch() != epoch {
		return empty, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	_, e = client.PutObjectV2(ctx, &tos.PutObjectV2Input{PutObjectBasicInput: tos.PutObjectBasicInput{Bucket: bucket, Key: key, ContentType: "application/zip", ContentLength: int64(len(data))}, Content: bytes.NewReader(data)})
	if e != nil {
		return empty, storageFailure(e)
	}
	if c.Epoch() != epoch {
		return empty, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	return SkillArchiveObject{URL: "https://" + bucket + ".tos-" + region + "." + storageDomain(cred.Provider) + "/" + key, Bucket: bucket}, nil
}
func storageFailure(err error) *dto.SafeError {
	code := "skillStorageFailed"
	status := tos.StatusCode(err)
	if status == 401 {
		code = "authRequired"
	}
	if status == 403 {
		code = "denied"
	}
	return dto.Failure(code, "platform", "readOnly", status)
}
