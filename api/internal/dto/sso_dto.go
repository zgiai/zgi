package dto

const SSOProviderCasdoor = "casdoor"

type SSOIdentity struct {
	Subject     string `json:"subject"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	CountryCode string `json:"country_code"`
}

type SSOProviderToken struct {
	Provider string `json:"provider"`
	IDToken  string `json:"id_token,omitempty"`
}

type SSOExchangeResult struct {
	Identity *SSOIdentity      `json:"identity"`
	Token    *SSOProviderToken `json:"token,omitempty"`
}
