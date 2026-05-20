package email

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderResetPasswordTemplateZhCNUsesChineseCopy(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	moduleRoot := filepath.Join(cwd, "..", "..")
	if err := os.Chdir(moduleRoot); err != nil {
		t.Fatalf("chdir module root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	htmlContent, err := renderResetPasswordTemplate("zh-CN", TemplateData{
		To:        "demo@example.com",
		Code:      "123456",
		LogoURL:   "https://example.com/logo.png",
		BrandName: "AIYOUNG",
	})
	if err != nil {
		t.Fatalf("renderResetPasswordTemplate returned error: %v", err)
	}

	if !strings.Contains(htmlContent, "请输入下方验证码") {
		t.Fatalf("expected chinese reset password copy, got %q", htmlContent)
	}

	if !strings.Contains(htmlContent, "版权所有") {
		t.Fatalf("expected chinese reset password footer, got %q", htmlContent)
	}
}

func TestChineseEmailTemplatesUseChineseCopy(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	moduleRoot := filepath.Join(cwd, "..", "..")
	if err := os.Chdir(moduleRoot); err != nil {
		t.Fatalf("chdir module root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	testCases := []struct {
		name        string
		template    string
		expected    string
		notExpected string
	}{
		{
			name:        "email code login zh-CN",
			template:    filepath.Join(moduleRoot, "templates", "email_code_login_mail_template_zh-CN.html"),
			expected:    "登录验证码",
			notExpected: "Login Verification Code",
		},
		{
			name:        "direct add member footer zh-CN",
			template:    filepath.Join(moduleRoot, "templates", "direct_add_member_mail_template_zh-CN.html"),
			expected:    "版权所有",
			notExpected: "All rights reserved",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content, err := os.ReadFile(tc.template)
			if err != nil {
				t.Fatalf("read template: %v", err)
			}

			text := string(content)
			if !strings.Contains(text, tc.expected) {
				t.Fatalf("expected template %s to contain %q", tc.template, tc.expected)
			}
			if strings.Contains(text, tc.notExpected) {
				t.Fatalf("expected template %s not to contain %q", tc.template, tc.notExpected)
			}
		})
	}
}
