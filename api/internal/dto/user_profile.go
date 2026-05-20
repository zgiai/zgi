package dto

import "time"

type ProfileExResponse struct {
	Account   AccountInfo `json:"account"`
	License   LicenseInfo `json:"license"`
	Role      string      `json:"role"`
	IsExpired bool        `json:"isExpired"`
}

type AccountInfo struct {
	Id        string    `json:"id"`
	Email     string    `json:"email"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LicenseInfo struct {
	Id        string    `json:"id"`
	AccountId string    `json:"accountId"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	ExpiredAt time.Time `json:"expiredAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
