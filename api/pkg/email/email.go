package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var (
	Cfg *config.Config
)

const resendAPIKeyEnv = "EMAIL_RESEND_API_KEY"

func Init(c *config.Config) {
	Cfg = c
}

type EmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Html    string   `json:"html"`
	Text    string   `json:"text,omitempty"`
}

type EmailResponse struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Created string `json:"created"`
	Error   string `json:"error"`
}

func SendEmail(to []string, subject, htmlContent string) error {
	return SendEmailWithBodyType(to, subject, htmlContent, "text/html")
}

func SendEmailWithBodyType(to []string, subject, body, bodyType string) error {
	if Cfg == nil {
		return fmt.Errorf("email service not initialized")
	}

	switch strings.ToLower(strings.TrimSpace(Cfg.Email.MailType)) {
	case "resend":
		return sendResendEmail(to, subject, body, bodyType)
	case "smtp":
		return sendSMTPEmail(to, subject, body, bodyType)
	default:
		return fmt.Errorf("unsupported email mail type: %s", Cfg.Email.MailType)
	}
}

func sendResendEmail(to []string, subject, body, bodyType string) error {
	if strings.TrimSpace(Cfg.Email.ResendAPIKey) == "" {
		return fmt.Errorf("%s is required", resendAPIKeyEnv)
	}

	logger.Info("preparing to send email",
		"provider", "resend",
		"recipient_count", len(to),
	)

	reqBody, err := BuildEmailRequest(
		Cfg.Email.MailDefaultSendFrom,
		to,
		subject,
		body,
		bodyType,
	)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("Failed to serialize request", err)
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	logger.Debug("email request prepared", "payload_bytes", len(jsonData))

	req, err := http.NewRequest("POST", Cfg.Email.ResendAPIURL+"/emails", bytes.NewBuffer(jsonData))

	if err != nil {
		logger.Error("Failed to create request", err)
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+Cfg.Email.ResendAPIKey)

	client := observability.HTTPClient(&http.Client{})
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to send request", err)
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response", err)
		return fmt.Errorf("failed to read response body: %w", err)
	}

	logger.Debug("email response received", "status_code", resp.StatusCode, "response_bytes", len(responseBody))

	if resp.StatusCode == http.StatusForbidden {
		var resendError struct {
			Name       string `json:"name"`
			Message    string `json:"message"`
			StatusCode int    `json:"statusCode"`
		}
		if err := json.Unmarshal(responseBody, &resendError); err != nil {
			logger.Error("Failed to parse error response", err)
			return fmt.Errorf("failed to unmarshal error response: %w", err)
		}
		logger.Error("Resend API error", fmt.Errorf("%s: %s (status %d)",
			resendError.Name,
			resendError.Message,
			resendError.StatusCode,
		))
		if resendError.Name == "validation_error" && strings.Contains(resendError.Message, "domain is not verified") {
			return fmt.Errorf("recipient domain not verified, please contact admin to add domain verification")
		}
		return fmt.Errorf("Resend API error: %s", resendError.Message)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		logger.Error("Email sending failed", fmt.Errorf("status code %d, response: %s", resp.StatusCode, string(responseBody)))
		return fmt.Errorf("failed to send email: status code %d, response: %s", resp.StatusCode, string(responseBody))
	}

	var emailResp EmailResponse
	if err := json.Unmarshal(responseBody, &emailResp); err != nil {
		logger.Error("Failed to parse response", err)
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if emailResp.Error != "" {
		err := fmt.Errorf("email service error: %s", emailResp.Error)
		logger.Error("Email service error", err)
		return err
	}

	logger.Info("Email sent successfully", fmt.Sprintf("ID: %s", emailResp.ID))
	return nil
}

func BuildEmailRequest(from string, to []string, subject, body, bodyType string) (*EmailRequest, error) {
	if strings.TrimSpace(bodyType) == "" {
		bodyType = "text/html"
	}

	req := &EmailRequest{
		From:    from,
		To:      to,
		Subject: subject,
	}

	switch strings.TrimSpace(bodyType) {
	case "text/html":
		req.Html = body
	case "text/plain":
		req.Text = body
	default:
		return nil, fmt.Errorf("unsupported email body_type: %s", bodyType)
	}

	return req, nil
}

func SendPasswordResetEmail(to string, newPassword string) error {
	subject := "密码重置通知"
	htmlContent := fmt.Sprintf(`
		<h2>密码重置通知</h2>
		<p>您的密码已经重置。新的临时密码是：</p>
		<p style="font-size: 18px; font-weight: bold; color: #333;">%s</p>
		<p>请使用此临时密码登录系统，并立即修改为您自己的密码。</p>
		<p>如果这不是您本人的操作，请立即联系管理员。</p>
	`, newPassword)

	return SendEmail([]string{to}, subject, htmlContent)
}

func SendWelcomeEmail(to string, username string) error {
	subject := "欢迎加入"
	htmlContent := fmt.Sprintf(`
		<h2>欢迎加入 ZGI Ginkit</h2>
		<p>亲爱的 %s：</p>
		<p>感谢您注册成为我们的用户！</p>
		<p>如果您有任何问题，请随时联系我们的支持团队。</p>
	`, username)

	return SendEmail([]string{to}, subject, htmlContent)
}

func SendResetPasswordMailTask(language, to, code string) error {
	if Cfg == nil {
		return fmt.Errorf("email service not initialized")
	}

	logger.Info(fmt.Sprintf("Start password reset mail to %s", to))
	startTime := time.Now()

	var htmlContent string
	var subject string
	var err error

	brandName := Cfg.Email.MailTemplateBrandName
	logoURL := Cfg.Email.MailTemplateLogoUrl

	if language == "zh-Hans" {
		htmlContent, err = renderResetPasswordTemplate("zh-CN", TemplateData{
			To:        to,
			Code:      code,
			LogoURL:   logoURL,
			BrandName: brandName,
		})
		if err != nil {
			logger.Error("Failed to render Chinese template", err)
			return fmt.Errorf("failed to render template: %w", err)
		}
		subject = fmt.Sprintf("%s验证码", brandName)
	} else {
		htmlContent, err = renderResetPasswordTemplate("en-US", TemplateData{
			To:        to,
			Code:      code,
			LogoURL:   logoURL,
			BrandName: brandName,
		})
		if err != nil {
			logger.Error("Failed to render English template", err)
			return fmt.Errorf("failed to render template: %w", err)
		}
		subject = fmt.Sprintf("%s Verification code", brandName)
	}

	logger.Debug("password reset email html rendered", "html_bytes", len(htmlContent))

	err = SendEmail([]string{to}, subject, htmlContent)
	if err != nil {
		logger.Error(fmt.Sprintf("Send password reset mail to %s failed", to), err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	endTime := time.Now()
	latency := endTime.Sub(startTime)
	logger.Info(fmt.Sprintf("Send password reset mail to %s succeeded: latency: %v", to, latency))

	return nil
}

type TemplateData struct {
	To        string
	Code      string
	LogoURL   string
	BrandName string
}

func loadTemplateFile(templateFileName string) ([]byte, error) {
	templatePaths := []string{
		filepath.Join("templates", templateFileName),
		filepath.Join("zgi-api-go", "templates", templateFileName),
		filepath.Join("..", "templates", templateFileName),
		filepath.Join("..", "zgi-api-go", "templates", templateFileName),
		filepath.Join("/app", "templates", templateFileName),
	}

	var templateContent []byte
	var err error

	for _, templatePath := range templatePaths {
		templateContent, err = os.ReadFile(templatePath)
		if err == nil {
			logger.Info(fmt.Sprintf("Template loaded from: %s", templatePath))
			break
		}
	}

	if err != nil {
		execPath, execErr := os.Executable()
		if execErr == nil {
			execDir := filepath.Dir(execPath)
			execTemplatePath := filepath.Join(execDir, "templates", templateFileName)
			templateContent, err = os.ReadFile(execTemplatePath)
			if err == nil {
				logger.Info(fmt.Sprintf("Template loaded from executable directory: %s", execTemplatePath))
			}
		}

		if err != nil {
			return nil, fmt.Errorf("failed to read template file %s from any location: %w", templateFileName, err)
		}
	}

	return templateContent, nil
}

func renderResetPasswordTemplate(language string, data TemplateData) (string, error) {
	var templateFileName string

	switch language {
	case "zh-CN":
		templateFileName = "reset_password_mail_template_zh-CN.html"
	case "en-US":
		templateFileName = "reset_password_mail_template_en-US.html"
	default:
		templateFileName = "reset_password_mail_template_en-US.html"
	}

	templateContent, err := loadTemplateFile(templateFileName)
	if err != nil {
		return "", err
	}

	content := string(templateContent)
	content = strings.ReplaceAll(content, "{{to}}", data.To)
	content = strings.ReplaceAll(content, "{{code}}", data.Code)
	content = strings.ReplaceAll(content, "{{logo_url}}", data.LogoURL)
	content = strings.ReplaceAll(content, "{{brand_name}}", data.BrandName)

	return content, nil
}

func SendInviteMemberMailTask(language, to, token, inviterName, workspaceName string) error {
	if Cfg == nil {
		return fmt.Errorf("email service not initialized")
	}

	logger.Info(fmt.Sprintf("Start invite member mail to %s", to))
	startTime := time.Now()

	var htmlContent string
	var subject string
	var err error

	brandName := Cfg.Email.MailTemplateBrandName
	logoURL := Cfg.Email.MailTemplateLogoUrl
	consoleWebURL := Cfg.Email.ConsoleWebURL

	activationURL := fmt.Sprintf("%s/activate?email=%s&token=%s", consoleWebURL, to, token)

	templateData := InviteTemplateData{
		To:            to,
		Token:         token,
		InviterName:   inviterName,
		WorkspaceName: workspaceName,
		LogoURL:       logoURL,
		BrandName:     brandName,
		ActivationURL: activationURL,
	}

	if language == "zh-Hans" {
		htmlContent, err = renderInviteMemberTemplate("zh-CN", templateData)
		if err != nil {
			logger.Error("Failed to render Chinese invite template", err)
			return fmt.Errorf("failed to render template: %w", err)
		}
		subject = fmt.Sprintf("邀请您加入 %s 工作空间", workspaceName)
	} else {
		htmlContent, err = renderInviteMemberTemplate("en-US", templateData)
		if err != nil {
			logger.Error("Failed to render English invite template", err)
			return fmt.Errorf("failed to render template: %w", err)
		}
		subject = fmt.Sprintf("You're invited to join %s workspace", workspaceName)
	}

	logger.Debug("invite email html rendered", "html_bytes", len(htmlContent))

	err = SendEmail([]string{to}, subject, htmlContent)
	if err != nil {
		logger.Error(fmt.Sprintf("Send invite member mail to %s failed", to), err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	endTime := time.Now()
	latency := endTime.Sub(startTime)
	logger.Info(fmt.Sprintf("Send invite member mail to %s succeeded: latency: %v", to, latency))

	return nil
}

type InviteTemplateData struct {
	To            string
	Token         string
	InviterName   string
	WorkspaceName string
	LogoURL       string
	BrandName     string
	ActivationURL string
}

func renderInviteMemberTemplate(language string, data InviteTemplateData) (string, error) {
	var templateFileName string

	switch language {
	case "zh-CN":
		templateFileName = "invite_member_mail_template_zh-CN.html"
	case "en-US":
		templateFileName = "invite_member_mail_template_en-US.html"
	default:
		templateFileName = "invite_member_mail_template_en-US.html"
	}

	templateContent, err := loadTemplateFile(templateFileName)
	if err != nil {
		return "", err
	}

	content := string(templateContent)
	content = strings.ReplaceAll(content, "{{to}}", data.To)
	content = strings.ReplaceAll(content, "{{token}}", data.Token)
	content = strings.ReplaceAll(content, "{{inviter_name}}", data.InviterName)
	content = strings.ReplaceAll(content, "{{workspace_name}}", data.WorkspaceName)
	content = strings.ReplaceAll(content, "{{logo_url}}", data.LogoURL)
	content = strings.ReplaceAll(content, "{{brand_name}}", data.BrandName)
	content = strings.ReplaceAll(content, "{{url}}", data.ActivationURL)

	return content, nil
}

type DirectAddMemberTemplateData struct {
	To             string
	GroupName      string
	DepartmentName string
	DepartmentText string
	LogoURL        string
	BrandName      string
	URL            string
}

func SendDirectAddMemberMail(language, to, groupName, departmentName, url string) error {
	if Cfg == nil {
		return fmt.Errorf("email service not initialized")
	}

	logger.Info(fmt.Sprintf("Start direct add member mail to %s", to))
	startTime := time.Now()

	brandName := Cfg.Email.MailTemplateBrandName
	logoURL := Cfg.Email.MailTemplateLogoUrl

	departmentText := ""
	if departmentName != "" {
		if language == "zh-Hans" || language == "zh-CN" {
			departmentText = fmt.Sprintf("，并分配到部门「%s」", departmentName)
		} else {
			departmentText = fmt.Sprintf(" and assigned you to the \"%s\" department", departmentName)
		}
	}

	data := DirectAddMemberTemplateData{
		To:             to,
		GroupName:      groupName,
		DepartmentName: departmentName,
		DepartmentText: departmentText,
		LogoURL:        logoURL,
		BrandName:      brandName,
		URL:            url,
	}

	htmlContent, subject, err := renderDirectAddMemberTemplate(language, data)
	if err != nil {
		logger.Error("Failed to render direct add member template", err)
		return fmt.Errorf("failed to render template: %w", err)
	}

	if err := SendEmail([]string{to}, subject, htmlContent); err != nil {
		logger.Error(fmt.Sprintf("Send direct add member mail to %s failed", to), err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	endTime := time.Now()
	latency := endTime.Sub(startTime)
	logger.Info(fmt.Sprintf("Send direct add member mail to %s succeeded: latency: %v", to, latency))

	return nil
}

func renderDirectAddMemberTemplate(language string, data DirectAddMemberTemplateData) (string, string, error) {
	var templateFileName string
	var subject string

	switch language {
	case "zh-Hans", "zh-CN":
		templateFileName = "direct_add_member_mail_template_zh-CN.html"
	default:
		templateFileName = "direct_add_member_mail_template_en-US.html"
	}

	templateContent, err := loadTemplateFile(templateFileName)
	if err != nil {
		return "", "", err
	}

	content := string(templateContent)
	content = strings.ReplaceAll(content, "{{to}}", data.To)
	content = strings.ReplaceAll(content, "{{group_name}}", data.GroupName)
	content = strings.ReplaceAll(content, "{{department_text}}", data.DepartmentText)
	content = strings.ReplaceAll(content, "{{logo_url}}", data.LogoURL)
	content = strings.ReplaceAll(content, "{{brand_name}}", data.BrandName)
	content = strings.ReplaceAll(content, "{{url}}", data.URL)

	if language == "zh-Hans" || language == "zh-CN" {
		subject = fmt.Sprintf("您已被添加到 %s", data.GroupName)
	} else {
		subject = fmt.Sprintf("You have been added to %s", data.GroupName)
	}

	return content, subject, nil
}

// SubscriptionExpiryTemplateData contains data for subscription expiry reminder email
type SubscriptionExpiryTemplateData struct {
	LogoURL    string
	BrandName  string
	PlanName   string
	DaysBefore string
	ExpiryDate string
}

// SendSubscriptionExpiryReminder sends a subscription expiry reminder email
func SendSubscriptionExpiryReminder(language, to, planName string, daysBefore int, expiryDate string) error {
	if Cfg == nil {
		return fmt.Errorf("email service not initialized")
	}

	logger.Info(fmt.Sprintf("Start subscription expiry reminder mail to %s", to))
	startTime := time.Now()

	var htmlContent string
	var subject string
	var err error

	brandName := Cfg.Email.MailTemplateBrandName
	logoURL := Cfg.Email.MailTemplateLogoUrl

	templateData := SubscriptionExpiryTemplateData{
		LogoURL:    logoURL,
		BrandName:  brandName,
		PlanName:   planName,
		DaysBefore: fmt.Sprintf("%d", daysBefore),
		ExpiryDate: expiryDate,
	}

	if language == "zh-Hans" || language == "zh-CN" {
		htmlContent, err = renderSubscriptionExpiryTemplate("zh-CN", templateData)
		if err != nil {
			logger.Error("Failed to render Chinese subscription expiry template", err)
			return fmt.Errorf("failed to render template: %w", err)
		}
		subject = fmt.Sprintf("订阅到期提醒 - %s", planName)
	} else {
		htmlContent, err = renderSubscriptionExpiryTemplate("en-US", templateData)
		if err != nil {
			logger.Error("Failed to render English subscription expiry template", err)
			return fmt.Errorf("failed to render template: %w", err)
		}
		subject = fmt.Sprintf("Subscription Expiry Reminder - %s", planName)
	}

	err = SendEmail([]string{to}, subject, htmlContent)
	if err != nil {
		logger.Error(fmt.Sprintf("Send subscription expiry reminder mail to %s failed", to), err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	endTime := time.Now()
	latency := endTime.Sub(startTime)
	logger.Info(fmt.Sprintf("Send subscription expiry reminder mail to %s succeeded: latency: %v", to, latency))

	return nil
}

func renderSubscriptionExpiryTemplate(language string, data SubscriptionExpiryTemplateData) (string, error) {
	var templateFileName string

	switch language {
	case "zh-CN":
		templateFileName = "subscription_expiry_reminder_zh-CN.html"
	case "en-US":
		templateFileName = "subscription_expiry_reminder_en-US.html"
	default:
		templateFileName = "subscription_expiry_reminder_en-US.html"
	}

	templateContent, err := loadTemplateFile(templateFileName)
	if err != nil {
		return "", err
	}

	content := string(templateContent)
	content = strings.ReplaceAll(content, "{{logo_url}}", data.LogoURL)
	content = strings.ReplaceAll(content, "{{brand_name}}", data.BrandName)
	content = strings.ReplaceAll(content, "{{plan_name}}", data.PlanName)
	content = strings.ReplaceAll(content, "{{days_before}}", data.DaysBefore)
	content = strings.ReplaceAll(content, "{{expiry_date}}", data.ExpiryDate)

	return content, nil
}
