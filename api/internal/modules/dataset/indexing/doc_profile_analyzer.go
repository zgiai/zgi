package indexing

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

type DocProfile struct {
	TotalElements int
	HeadingCount  int
	IsSectionLike bool
}

func (p DocProfile) String() string {
	return fmt.Sprintf("TotalElements: %d, HeadingCount: %d, IsSectionLike: %v", p.TotalElements, p.HeadingCount, p.IsSectionLike)
}

type DocProfileAnalyzer struct{}

func NewDocProfileAnalyzer() *DocProfileAnalyzer {
	return &DocProfileAnalyzer{}
}

func (a *DocProfileAnalyzer) Analyze(output *dto.ExtractOutput) DocProfile {
	if output == nil {
		return DocProfile{}
	}

	total := len(output.Elements)
	headings := 0

	for _, elem := range output.Elements {
		if strings.ToLower(elem.Type) == "heading" {
			headings++
		}
	}

	isSectionLike := false
	if total > 0 {
		headingRatio := float64(headings) / float64(total)
		// If at least 3 headings or ratio is significant, consider it section-like.
		if headings >= 3 || (headings > 0 && headingRatio > 0.05) {
			isSectionLike = true
		}
	}

	return DocProfile{
		TotalElements: total,
		HeadingCount:  headings,
		IsSectionLike: isSectionLike,
	}
}
