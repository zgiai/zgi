package util

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"crypto/sha256"
	"encoding/hex"
	"errors"

	"math/rand"

	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/zgiai/ginext/pkg/logger"
	redisUtil "github.com/zgiai/ginext/pkg/redis"
	"go.uber.org/zap"
)

// RateLimiter handles rate limiting
type RateLimiter struct {
	Prefix      string
	TimeWindow  int64 // seconds
	MaxAttempts int64
}

// NewRateLimiter creates a new RateLimiter instance
func NewRateLimiter(prefix string, maxAttempts int64, timeWindow int64) *RateLimiter {
	return &RateLimiter{
		Prefix:      prefix,
		TimeWindow:  timeWindow,  // 60 second window
		MaxAttempts: maxAttempts, // maximum 5 times
	}
}

func (r *RateLimiter) getKey(email string) string {
	return fmt.Sprintf("rate_limit:%s:%s", r.Prefix, strings.ToLower(strings.TrimSpace(email)))
}

// IsRateLimited determine if rate limited
func (r *RateLimiter) IsRateLimited(ctx context.Context, email string) (bool, error) {
	key := r.getKey(email)
	currentTime := time.Now().Unix()
	windowStartTime := currentTime - r.TimeWindow

	// Remove scores outside the window
	client := redisUtil.GetClient()
	_, err := client.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(windowStartTime, 10)).Result()
	if err != nil {
		return false, err
	}

	// Count operations within the window
	attempts, err := client.ZCard(ctx, key).Result()
	if err != nil {
		return false, err
	}

	if attempts >= r.MaxAttempts {
		return true, nil
	}
	return false, nil
}

// IncrementRateLimit increments the rate limit counter for an email
func (r *RateLimiter) IncrementRateLimit(email string) {
	key := r.getKey(email)
	ctx := context.Background()
	now := time.Now()
	pipe := redisUtil.GetClient().Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.Unix()),
		Member: fmt.Sprintf("%d:%s", now.UnixNano(), uuid.NewString()),
	})
	pipe.Expire(ctx, key, time.Duration(r.TimeWindow)*time.Second)
	_, err := pipe.Exec(ctx)
	if err != nil {
		logger.Warn("Failed to increment rate limit",
			zap.String("rate_limit_type", r.Prefix),
			zap.Error(err),
		)
	}
}

// IsLoginRateLimited checks if login is rate limited for an email
func IsLoginRateLimited(email string) bool {
	key := fmt.Sprintf("login:%s", email)
	ctx := context.Background()
	count, err := redisUtil.GetClient().Get(ctx, key).Int()
	if err != nil {
		return false
	}
	return count >= config.MaxLoginAttempts
}

// IncrLoginFailCount increments the login failure count
func IncrLoginFailCount(email string) {
	key := fmt.Sprintf("login:%s", email)
	ctx := context.Background()
	pipe := redisUtil.GetClient().Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Duration(config.RateLimitWindow)*time.Minute)
	_, err := pipe.Exec(ctx)
	if err != nil {
		logger.Warn("Failed to increment login failure count", zap.String("rate_limit_type", "login"), zap.Error(err))
	}
}

// ResetLoginFailCount resets the login failure count
func ResetLoginFailCount(email string) {
	key := fmt.Sprintf("login:%s", email)
	ctx := context.Background()
	redisUtil.GetClient().Del(ctx, key)
}

// IncrForgotPasswordErrorCount increments the forgot password error count
func IncrForgotPasswordErrorCount(email string) {
	key := forgotPasswordErrorRateLimitKey(email)
	ctx := context.Background()
	pipe := redisUtil.GetClient().Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Duration(config.RateLimitWindow)*time.Minute)
	_, err := pipe.Exec(ctx)
	if err != nil {
		logger.Warn("Failed to increment forgot password error count",
			zap.String("rate_limit_type", "forgot_password"),
			zap.Error(err),
		)
	}
}

// GetForgotPasswordErrorCount gets the forgot password error count
func GetForgotPasswordErrorCount(email string) int64 {
	key := forgotPasswordErrorRateLimitKey(email)
	ctx := context.Background()
	count, err := redisUtil.GetClient().Get(ctx, key).Int64()
	if err != nil {
		return 0
	}
	return count
}

func forgotPasswordErrorRateLimitKey(email string) string {
	return fmt.Sprintf("forgot_password_error_rate_limit:%s", strings.ToLower(strings.TrimSpace(email)))
}

// ValidPassword checks if a password meets the requirements
func ValidPassword(password string) bool {
	re := regexp.MustCompile(`^(?=.*[a-zA-Z])(?=.*\d).{8,}$`)
	return re.MatchString(password)
}

// Email validate email format
func Email(email string) (string, error) {
	pattern := `^[\w\.!#$%&'*+\-/=?^_` + "`" + `{|}~]+@([\w-]+\.)+[\w-]{2,}$`
	matched, _ := regexp.MatchString(pattern, email)
	if matched {
		return email, nil
	}
	return "", fmt.Errorf("%s is not a valid email", email)
}

// UUIDValue validate UUID format
func UUIDValue(value string) (string, error) {
	if value == "" {
		return value, nil
	}
	_, err := uuid.Parse(value)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid uuid", value)
	}
	return value, nil
}

// Alphanumeric validate only contains letters, numbers and underscores
func Alphanumeric(value string) (string, error) {
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, value)
	if matched {
		return value, nil
	}
	return "", fmt.Errorf("%s is not a valid alphanumeric value", value)
}

// TimestampValue validate and return int64 timestamp
func TimestampValue(ts string) (int64, error) {
	val, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || val < 0 {
		return 0, fmt.Errorf("%s is not a valid timestamp", ts)
	}
	return val, nil
}

// StrLen limit string maximum length
func StrLen(value string, maxLength int, argument string) (string, error) {
	if len(value) > maxLength {
		return "", fmt.Errorf("Invalid %s: %s. %s cannot exceed length %d", argument, value, argument, maxLength)
	}
	return value, nil
}

// FloatRange limit float within range
func FloatRange(value float64, low, high float64, argument string) (float64, error) {
	if value < low || value > high {
		return 0, fmt.Errorf("Invalid %s: %v. %s must be within the range %v - %v", argument, value, argument, low, high)
	}
	return value, nil
}

// DatetimeString validate if string conforms to time format
func DatetimeString(value, format, argument string) (string, error) {
	_, err := time.Parse(format, value)
	if err != nil {
		return "", fmt.Errorf("Invalid %s: %s. %s must be conform to the format %s", argument, value, argument, format)
	}
	return value, nil
}

// Timezone validate timezone string
func Timezone(tz string) (string, error) {
	// Go doesn't have available_timezones, need to use IANA data
	if tz == "" {
		return "", errors.New("timezone string is empty")
	}
	_, err := time.LoadLocation(tz)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid timezone", tz)
	}
	return tz, nil
}

// GenerateString generate random string
func GenerateString(n int) string {
	lettersDigits := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	sb := strings.Builder{}
	for i := 0; i < n; i++ {
		sb.WriteByte(lettersDigits[rand.Intn(len(lettersDigits))])
	}
	return sb.String()
}

// GenerateTextHash generate text hash
func GenerateTextHash(text string) string {
	hashText := text + "None"
	hash := sha256.Sum256([]byte(hashText))
	return hex.EncodeToString(hash[:])
}

func HttpGetJSON(
	ctx context.Context,
	baseURL, path string,
	params map[string]string,
	headers map[string]string, // Added header parameter
	result interface{},
) error {
	// Construct URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	u.Path = path
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	// Construct request
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status: %d", resp.StatusCode)
	}

	// Parse JSON
	return json.NewDecoder(resp.Body).Decode(result)
}

func GenerateRandomNumberString(length int) string {
	if length <= 0 {
		return ""
	}
	rand.Seed(time.Now().UnixNano())
	digits := "0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}
