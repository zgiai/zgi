package routing

import "github.com/zgiai/ginext/internal/contracts"

func normalizeProfile(profile contracts.ParseProfile) contracts.ParseProfile {
	switch profile {
	case contracts.ParseProfileAuto,
		contracts.ParseProfileHighQuality,
		contracts.ParseProfileFast,
		contracts.ParseProfileLocalFirst,
		contracts.ParseProfileFastPreview,
		contracts.ParseProfileLayoutFirst,
		contracts.ParseProfileTextFirst,
		contracts.ParseProfileDatasetIndex:
		return profile
	case "":
		return contracts.ParseProfileAuto
	default:
		return contracts.ParseProfileAuto
	}
}

func profileAllowsRemote(profile contracts.ParseProfile) bool {
	switch normalizeProfile(profile) {
	case contracts.ParseProfileLocalFirst:
		return false
	default:
		return true
	}
}
