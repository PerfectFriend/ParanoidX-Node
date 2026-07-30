// Package radio provides the radio streaming and scheduling system
package radio

import (
	"fmt"
	"log/slog"

	"ParanoidX/internal/ai"
)

type AIContentGenerator struct {
	Client *ai.Client
}


// NewAIContentGenerator handles the NewAIContentGenerator HTTP request.
func NewAIContentGenerator(client *ai.Client) *AIContentGenerator {
	return &AIContentGenerator{Client: client}
}


// GenerateNewsBulletin handles the GenerateNewsBulletin HTTP request.
func (g *AIContentGenerator) GenerateNewsBulletin(topic string) (string, error) {
	if g.Client == nil {
		return "[AI news unavailable]", nil
	}
	prompt := fmt.Sprintf(`Write a 30-second radio news bulletin about: %s

Style: Professional news anchor. Concise, factual, broadcast-quality. End with "This has been your news update from Saint Mary Liberty Island."`, topic)
	resp, err := g.Client.Generate(prompt, "You are a radio news anchor. Speak clearly and authoritatively.", ai.Options{Temperature: 0.7, NumPredict: 200})
	if err != nil {
		return "", err
	}
	return resp.Response, nil
}


// GenerateAdvertisement handles the GenerateAdvertisement HTTP request.
func (g *AIContentGenerator) GenerateAdvertisement(product string) (string, error) {
	if g.Client == nil {
		return "[AI ad unavailable]", nil
	}
	prompt := fmt.Sprintf(`Write a 15-second radio advertisement for: %s

Style: Energetic, persuasive, memorable. End with a call to action. Island-appropriate content.`, product)
	resp, err := g.Client.Generate(prompt, "You are a radio advertising voice actor.", ai.Options{Temperature: 0.8, NumPredict: 150})
	if err != nil {
		return "", err
	}
	return resp.Response, nil
}


// GenerateKingDecree handles the GenerateKingDecree HTTP request.
func (g *AIContentGenerator) GenerateKingDecree(topic string) (string, error) {
	if g.Client == nil {
		return "[King's decree unavailable]", nil
	}
	prompt := fmt.Sprintf(`Write a royal decree from the King of Saint Mary Liberty Island about: %s

Style: Formal, majestic, authoritative. Begin with "Hear ye, hear ye!" and end with "By my hand, on this day."`, topic)
	resp, err := g.Client.Generate(prompt, "You are a king issuing a royal decree. Speak with authority and wisdom.", ai.Options{Temperature: 0.7, NumPredict: 200})
	if err != nil {
		return "", err
	}
	return resp.Response, nil
}


// GenerateStationFill handles the GenerateStationFill HTTP request.
func (g *AIContentGenerator) GenerateStationFill(stationID string, durationSec int) ([]string, error) {
	if g.Client == nil {
		return []string{"[Content generation unavailable]"}, nil
	}
	items := durationSec / 30
	if items < 1 {
		items = 1
	}
	if items > 10 {
		items = 10
	}
	var results []string
	topics := []string{"the island community", "local economy update", "silver standard report", "upcoming events", "wisdom of the day"}
	for i := 0; i < items; i++ {
		topic := topics[i%len(topics)]
		text, err := g.GenerateNewsBulletin(topic)
		if err != nil {
			slog.Warn("ai content generation failed", "error", err, "topic", topic)
			text = fmt.Sprintf("[Segment %d: %s]", i+1, topic)
		}
		results = append(results, text)
	}
	return results, nil
}
