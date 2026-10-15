package zgimarket

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// maxFreePluginMembers bounds how many member listings are read to tell a plugin is free.
const maxFreePluginMembers = 20

// marketPrice is the part of a listing price that tells whether it costs anything.
type marketPrice struct {
	OfferingSlug    string `json:"offering_slug"`
	UnitPriceMicros int64  `json:"unit_price_micros"`
}

type describedListing struct {
	ID        uint64        `json:"id"`
	Kind      string        `json:"kind"`
	Prices    []marketPrice `json:"prices"`
	Offerings []struct {
		ID   uint64 `json:"id"`
		Slug string `json:"slug"`
	} `json:"offerings"`
	Plugin *struct {
		Members []struct {
			Kind string `json:"kind"`
			Slug string `json:"slug"`
		} `json:"members"`
	} `json:"plugin"`
}

// pricesAreFree reports whether every price that applies costs nothing. A listing without a
// published price is not taken for free: its cost is unknown. offeringSlug, when given, limits
// the prices to that offering and to the ones that name none.
func pricesAreFree(prices []marketPrice, offeringSlug string) bool {
	applied := 0
	for _, price := range prices {
		if offeringSlug != "" && price.OfferingSlug != "" && price.OfferingSlug != offeringSlug {
			continue
		}
		if price.UnitPriceMicros != 0 {
			return false
		}
		applied++
	}
	return applied > 0
}

// describe reads one listing as the caller; ok is false when it cannot be read.
func (p *Proxy) describe(ctx context.Context, who caller, kind, slug string) (describedListing, bool) {
	if !activationKinds[kind] || !marketSlugPattern.MatchString(slug) {
		return describedListing{}, false
	}
	result, err := p.call(ctx, who, http.MethodGet, "listings/"+kind+"/"+slug, nil, nil, answerBodyLimit)
	if err != nil || result.status != http.StatusOK {
		return describedListing{}, false
	}
	var described struct {
		Data describedListing `json:"data"`
	}
	if json.Unmarshal(result.content, &described) != nil || described.Data.ID == 0 {
		return describedListing{}, false
	}
	return described.Data, true
}

// freeToActivate reports whether the listing named by kind and slug is the one with listingID and
// costs nothing to activate and to use: every price of it, of the offering asked for, and of each
// member of a plugin is zero. A member activates such a listing without an administrator.
func (p *Proxy) freeToActivate(ctx context.Context, who caller, kind, slug string, listingID, offeringID uint64) bool {
	listing, ok := p.describe(ctx, who, kind, slug)
	if !ok || listing.ID != listingID || listing.Kind != kind {
		return false
	}
	offeringSlug := ""
	if offeringID != 0 {
		for _, offering := range listing.Offerings {
			if offering.ID == offeringID {
				offeringSlug = offering.Slug
			}
		}
		if offeringSlug == "" {
			return false
		}
	} else if kind == "api" {
		// An API is activated per offering.
		return false
	}
	if !pricesAreFree(listing.Prices, offeringSlug) {
		return false
	}
	if kind != "plugin" || listing.Plugin == nil {
		return true
	}
	if len(listing.Plugin.Members) > maxFreePluginMembers {
		return false
	}
	for _, member := range listing.Plugin.Members {
		described, ok := p.describe(ctx, who, member.Kind, member.Slug)
		if !ok || !pricesAreFree(described.Prices, "") {
			return false
		}
	}
	return true
}

// freeSubscription lets a member activate a free listing: given the body of POST subscriptions,
// it reports whether the listing is free and returns the body the market takes. The page names
// the listing's kind and slug beside its id, so it can be read; the market does not take them.
func (p *Proxy) freeSubscription(ctx context.Context, who caller, body []byte) ([]byte, bool) {
	var input struct {
		ListingID   uint64 `json:"listing_id"`
		OfferingID  uint64 `json:"offering_id"`
		ListingKind string `json:"listing_kind"`
		ListingSlug string `json:"listing_slug"`
	}
	if json.Unmarshal(body, &input) != nil || input.ListingID == 0 {
		return nil, false
	}
	switch input.ListingKind {
	case "api", "mcp", "plugin":
	default:
		return nil, false
	}
	if !p.freeToActivate(ctx, who, input.ListingKind, input.ListingSlug, input.ListingID, input.OfferingID) {
		return nil, false
	}
	forwarded := `{"listing_id":` + strconv.FormatUint(input.ListingID, 10)
	if input.OfferingID != 0 {
		forwarded += `,"offering_id":` + strconv.FormatUint(input.OfferingID, 10)
	}
	return []byte(forwarded + "}"), true
}
