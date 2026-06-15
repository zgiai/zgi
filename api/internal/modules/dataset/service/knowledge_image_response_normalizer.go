package service

import (
	"fmt"

	"github.com/zgiai/zgi/api/internal/dto"
)

func normalizeHitTestingResponseKnowledgeImageURLs(response *dto.HitTestingResponse, filesBaseURL string) error {
	if response == nil {
		return nil
	}

	for i := range response.Records {
		content, err := NormalizeKnowledgeImageURLs(response.Records[i].Segment.Content, filesBaseURL)
		if err != nil {
			return fmt.Errorf("normalize segment content knowledge image URLs: %w", err)
		}
		response.Records[i].Segment.Content = content

		signContent, err := NormalizeKnowledgeImageURLs(response.Records[i].Segment.SignContent, filesBaseURL)
		if err != nil {
			return fmt.Errorf("normalize segment sign content knowledge image URLs: %w", err)
		}
		response.Records[i].Segment.SignContent = signContent

		for j := range response.Records[i].ChildChunks {
			childContent, err := NormalizeKnowledgeImageURLs(response.Records[i].ChildChunks[j].Content, filesBaseURL)
			if err != nil {
				return fmt.Errorf("normalize child chunk content knowledge image URLs: %w", err)
			}
			response.Records[i].ChildChunks[j].Content = childContent
		}
	}

	return nil
}
