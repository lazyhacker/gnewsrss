package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// RSS struct to parse RSS feed

type RSS struct {
	Channel Channel `xml:"channel"`
}

type Channel struct {
	Title string `xml:"title"`
	Items []Item `xml:"item"`
}

type Item struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

func main() {
	// URL of the RSS feed
	feedURLs := []string{
		"https://news.google.com/rss",
		//"https://news.google.com/rss/topics/CAAqJggKIiBDQkFTRWdvSUwyMHZNRGx1YlY4U0FtVnVHZ0pWVXlnQVAB?hl=en-US&gl=US&ceid=US%3Aen", // World
		//"https://news.google.com/rss/topics/CAAqJggKIiBDQkFTRWdvSUwyMHZNRGx6TVdZU0FtVnVHZ0pWVXlnQVAB?hl=en-US&gl=US&ceid=US%3Aen", // Business
	}

	var prompt strings.Builder

	for _, u := range feedURLs {
		// Fetch RSS feed
		resp, err := http.Get(u)
		if err != nil {
			log.Fatalf("Error fetching RSS feed: %v", err)
		}
		defer resp.Body.Close()
		// Read and parse RSS feed
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Error reading RSS feed: %v", err)
		}

		var rss RSS
		if err := xml.Unmarshal(body, &rss); err != nil {
			log.Fatalf("Error parsing RSS feed: %v", err)
		}

		// Print feed title
		//fmt.Println("Feed Title:", rss.Channel.Title)

		//prompt.WriteString(fmt.Sprintf("Feed Title: %v\n", rss.Channel.Title))

		// Print the titles and links of the latest articles
		for _, item := range rss.Channel.Items {
			//fmt.Printf("- %s (%s)\n", item.Title, item.Link)
			//fmt.Printf("%s\n", item.Title)
			prompt.WriteString(fmt.Sprintf("Headline: %v\nLink: %v\n\n", item.Title, item.Link))
		}
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		log.Fatalf("Error generating client. %v", err)

	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")
	model.SetTemperature(1)
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(8192)
	model.ResponseMIMEType = "application/json"
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text("You are a news filter who will be given a list of news headlines and their respective link to filter out any headlines that is are political in nature.  Any headline that mentions Trump are considered political and should be removed.  Also filter out any headlines that include individuals such as Elon Musk who are political figures even though they aren't politicians. Also filter out individuals who are known wealthy politicial donors.")},
	}
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"headlines": &genai.Schema{
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"title": &genai.Schema{
							Type: genai.TypeString,
						},
						"link": &genai.Schema{
							Type: genai.TypeString,
						},
					},
				},
			},
		},
	}

	fmt.Println("Calling Gemini")
	fmt.Printf("Prompt = %s", prompt.String())
	chat := model.StartChat()
	res, err := chat.SendMessage(ctx, genai.Text(prompt.String()))
	if err != nil {
		log.Fatalf("Error sending message: %v", err)
	}

	f, err := os.Create(filepath.Join("docs", "headlines.json"))
	if err != nil {
		fmt.Println("Error creating or opening the file:", err)
		return
	}
	defer f.Close()

	for _, part := range res.Candidates[0].Content.Parts {
		//fmt.Printf("%v\n", part)
		fmt.Println("Writing to file.")
		_, err := f.WriteString(fmt.Sprintf("%v\n", part))
		if err != nil {
			panic(err)
		}
	}
}
