package workspacecheckpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// S3Config identifies an approved, private S3-compatible checkpoint bucket.
// Credentials come from the standard AWS credential chain, never from a
// persisted Binding or a request supplied by an Agent.
type S3Config struct {
	Bucket       string
	Region       string
	Endpoint     string
	Prefix       string
	UsePathStyle bool
}

type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
}

var _ agentexecution.WorkspaceCheckpointStore = (*S3Store)(nil)

func NewS3Store(ctx context.Context, settings S3Config) (*S3Store, error) {
	settings.Bucket = strings.TrimSpace(settings.Bucket)
	settings.Region = strings.TrimSpace(settings.Region)
	settings.Endpoint = strings.TrimSpace(settings.Endpoint)
	settings.Prefix = strings.Trim(settings.Prefix, "/")
	if settings.Bucket == "" || settings.Region == "" {
		return nil, fmt.Errorf("%w: checkpoint S3 bucket and region are required", agentexecution.ErrInvalidRequest)
	}
	if settings.Prefix != "" {
		for _, part := range strings.Split(settings.Prefix, "/") {
			if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\\x00") {
				return nil, fmt.Errorf("%w: unsafe checkpoint S3 prefix", agentexecution.ErrInvalidRequest)
			}
		}
	}
	if settings.Endpoint != "" {
		endpoint, err := url.Parse(settings.Endpoint)
		if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Path != "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
			return nil, fmt.Errorf("%w: checkpoint S3 endpoint must be an origin URL", agentexecution.ErrInvalidRequest)
		}
		if endpoint.Scheme == "http" && endpoint.Hostname() != "localhost" && endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "::1" {
			return nil, fmt.Errorf("%w: remote checkpoint S3 endpoint requires HTTPS", agentexecution.ErrInvalidRequest)
		}
	}
	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(settings.Region))
	if err != nil {
		return nil, fmt.Errorf("configure checkpoint S3 client: %w", err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = settings.UsePathStyle
		if settings.Endpoint != "" {
			options.BaseEndpoint = aws.String(settings.Endpoint)
		}
	})
	return &S3Store{client: client, bucket: settings.Bucket, prefix: settings.Prefix}, nil
}

func (s *S3Store) objectKey(key string) string {
	if s.prefix == "" {
		return key + ".tar"
	}
	return s.prefix + "/" + key + ".tar"
}

func (s *S3Store) Save(ctx context.Context, key string, source io.Reader) error {
	if !safeKey.MatchString(key) || source == nil {
		return agentexecution.ErrInvalidRequest
	}
	file, err := os.CreateTemp("", "zgi-checkpoint-*.tar")
	if err != nil {
		return fmt.Errorf("create checkpoint upload spool: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(&contextReader{ctx: ctx, reader: source}, maxCheckpointBytes+1))
	if size > maxCheckpointBytes {
		return agentexecution.ErrOutputLimit
	}
	if err != nil {
		return fmt.Errorf("spool workspace checkpoint: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync workspace checkpoint spool: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	checksum := hex.EncodeToString(digest.Sum(nil))
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(s.objectKey(key)), Body: file,
		ContentLength: aws.Int64(size), IfNoneMatch: aws.String("*"),
		Metadata: map[string]string{"sha256": checksum},
	})
	if isS3Code(err, "PreconditionFailed", "ConditionalRequestConflict") {
		// A retry after a committed write must see precisely the same bytes.
		reader, openErr := s.Open(ctx, key)
		if openErr != nil {
			return openErr
		}
		_, verifyErr := io.Copy(io.Discard, reader)
		verifyErr = errors.Join(verifyErr, reader.Close())
		if verifyErr != nil {
			return verifyErr
		}
		if existing, ok := reader.(*s3VerifyingReader); !ok || existing.size != size || existing.expected != checksum {
			return fmt.Errorf("%w: checkpoint key already contains different bytes", agentexecution.ErrIdempotencyConflict)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("publish workspace checkpoint: %w", err)
	}
	return nil
}

func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if !safeKey.MatchString(key) {
		return nil, agentexecution.ErrInvalidRequest
	}
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.objectKey(key))})
	if isS3Code(err, "NoSuchKey", "NotFound") {
		return nil, agentexecution.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace checkpoint: %w", err)
	}
	size := aws.ToInt64(result.ContentLength)
	checksum := result.Metadata["sha256"]
	if result.ContentLength == nil || size < 0 || size > maxCheckpointBytes || !safeKey.MatchString(checksum) {
		_ = result.Body.Close()
		return nil, fmt.Errorf("%w: invalid checkpoint object metadata", agentexecution.ErrBackendUnavailable)
	}
	return &s3VerifyingReader{ctx: ctx, body: result.Body, size: size, remaining: size, expected: checksum, hash: sha256.New()}, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if !safeKey.MatchString(key) {
		return agentexecution.ErrInvalidRequest
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(s.objectKey(key))})
	return err
}

type s3VerifyingReader struct {
	ctx       context.Context
	body      io.ReadCloser
	size      int64
	remaining int64
	expected  string
	hash      hash.Hash
	verified  bool
}

func (r *s3VerifyingReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.verified {
		return 0, io.EOF
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if int64(len(buffer)) > r.remaining+1 {
		buffer = buffer[:r.remaining+1]
	}
	n, err := r.body.Read(buffer)
	if n > 0 {
		r.remaining -= int64(n)
		_, _ = r.hash.Write(buffer[:n])
	}
	if r.remaining < 0 || (errors.Is(err, io.EOF) && r.remaining != 0) {
		return n, fmt.Errorf("%w: workspace checkpoint size mismatch", agentexecution.ErrBackendUnavailable)
	}
	if r.remaining == 0 && (n > 0 || errors.Is(err, io.EOF)) {
		// Read one more byte to reject an object longer than its recorded size.
		var extra [1]byte
		extraCount, extraErr := io.ReadFull(r.body, extra[:])
		if extraCount != 0 || (extraErr != nil && !errors.Is(extraErr, io.EOF)) || hex.EncodeToString(r.hash.Sum(nil)) != r.expected {
			return n, fmt.Errorf("%w: workspace checkpoint checksum mismatch", agentexecution.ErrBackendUnavailable)
		}
		r.verified = true
		return n, io.EOF
	}
	return n, err
}

func (r *s3VerifyingReader) Close() error { return r.body.Close() }

func isS3Code(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	for _, code := range codes {
		if apiErr.ErrorCode() == code {
			return true
		}
	}
	return false
}
