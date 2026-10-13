package console

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRemoteRegisterOrganizationIsSynchronousAndIdempotent(t *testing.T) {
	requestObserved := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestObserved = true
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, apiPrefix+pathOrgRegister, r.URL.Path)
		require.Equal(t, "registration-organization:org-1", r.Header.Get("Idempotency-Key"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"owner_email":"owner@example.com"`)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()

	remote := NewRemote(server.URL, "test-signing-key")
	err := remote.RegisterOrganizationSync(t.Context(), &RegisterOrganizationRequest{
		OrganizationID: "org-1", Name: "Organization", OwnerEmail: "owner@example.com", CreatedAt: time.Now().UTC(),
	})

	require.NoError(t, err)
	require.True(t, requestObserved, "the call must finish only after the response is observed")
}

func TestRemoteRegisterOrganizationPropagatesHTTPAndNetworkErrors(t *testing.T) {
	t.Run("non-2xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"upstream unavailable"}`))
		}))
		defer server.Close()
		remote := NewRemote(server.URL, "")

		err := remote.RegisterOrganizationSync(t.Context(), &RegisterOrganizationRequest{OrganizationID: "org-1"})

		var apiErr *ConsoleAPIError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
		require.Equal(t, "upstream unavailable", apiErr.Message)
	})

	t.Run("network", func(t *testing.T) {
		remote := NewRemote("http://console.invalid", "")
		remote.httpClient = &http.Client{Transport: registrationRoundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		})}

		err := remote.RegisterOrganizationSync(context.Background(), &RegisterOrganizationRequest{OrganizationID: "org-1"})

		require.ErrorContains(t, err, "dial failed")
	})
}

func TestRemoteRejectsRedirectsBeforeSendingControlPlaneCredentials(t *testing.T) {
	redirectTargetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetCalled = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer redirector.Close()

	remote := NewRemote(redirector.URL, "secret")
	err := remote.RegisterOrganizationSync(t.Context(), &RegisterOrganizationRequest{OrganizationID: "org-1"})

	require.ErrorContains(t, err, "refusing Console API redirect to different host")
	require.False(t, redirectTargetCalled)
}

func TestRemoteRegisterOrganizationOnlyAcceptsExplicitAlreadyExistsConflict(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "already registered", body: `{"code":"organization_already_exists","message":"organization already exists"}`},
		{name: "ambiguous conflict", body: `{"code":"conflict","message":"name conflict"}`, wantErr: true},
		{name: "owner email conflict", body: `{"code":"already_exists","message":"owner email already exists"}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			remote := NewRemote(server.URL, "")

			err := remote.RegisterOrganizationSync(t.Context(), &RegisterOrganizationRequest{OrganizationID: "org-1"})

			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRemoteNotifyOfficialSignupUsesStableIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, apiPrefix+pathSignupGift, r.URL.Path)
		require.Equal(t, "registration-signup:org-1:account-1", r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"organization_id":"org-1","account_id":"account-1","already_granted":true}}`))
	}))
	defer server.Close()
	remote := NewRemote(server.URL, "")

	response, err := remote.NotifyOfficialSignup(t.Context(), &NotifyOfficialSignupRequest{
		OrganizationID: "org-1",
		AccountID:      "account-1",
	})

	require.NoError(t, err)
	require.NotNil(t, response)
	require.True(t, response.AlreadyGranted)
}

// TestRemoteSignRequestSendsInternalSecret verifies the configured Console
// credential is sent raw in the existing X-ZGI-Internal-Secret slot, with the
// HMAC signature kept for the shared-secret compatibility path.
func TestRemoteSignRequestSendsInternalSecret(t *testing.T) {
	var gotSecret, gotSignature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-ZGI-Internal-Secret")
		gotSignature = r.Header.Get("X-Internal-Signature")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()

	remote := NewRemote(server.URL, "test-signing-key")
	if err := remote.RegisterOrganizationSync(t.Context(), &RegisterOrganizationRequest{OrganizationID: "org-1"}); err != nil {
		t.Fatalf("RegisterOrganizationSync() error = %v", err)
	}
	require.Equal(t, "test-signing-key", gotSecret)
	require.NotEmpty(t, gotSignature)
}

type registrationRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f registrationRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
