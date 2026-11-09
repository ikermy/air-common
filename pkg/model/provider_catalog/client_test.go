package provider_catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchOpenAIModelsReturnsGeneralModelsOnly(t *testing.T) {
	client := &Client{HTTPClient: &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"data": [
                        {"id":"gpt-5"},
                        {"id":"gpt-5.6-Luna"},
                        {"id":"gpt-4.1-mini"},
                        {"id":"gpt-realtime"},
                        {"id":"gpt-realtime-mini"},
                        {"id":"gpt-4o-mini-tts"},
                        {"id":"gpt-4o-transcribe"},
                        {"id":"text-embedding-3-large"},
                        {"id":"moderation-latest"}
                    ]
				}`)),
			}, nil
		}),
	}}

	got, err := client.generalOpenAIModels(context.Background(), "urls-api-key")
	if err != nil {
		t.Fatalf("generalOpenAIModels() returned error: %v", err)
	}

	want := []string{"gpt-5", "gpt-5.6-Luna", "gpt-4.1-mini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generalOpenAIModels() = %v, want %v", got, want)
	}

}

func TestFetchOpenAIModelsReturnsRealtimeModelsOnly(t *testing.T) {
	client := &Client{HTTPClient: &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"data": [
						{"id":"gpt-5"},
						{"id":"gpt-realtime"},
						{"id":"gpt-realtime-mini"},
						{"id":"gpt-4o-mini-tts"},
						{"id":"gpt-4o-transcribe"}
					]
				}`)),
			}, nil
		}),
	}}

	got, err := client.realtimeOpenAIModels(context.Background(), "urls-api-key")
	if err != nil {
		t.Fatalf("realtimeOpenAIModels() returned error: %v", err)
	}

	want := []string{"gpt-realtime", "gpt-realtime-mini"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("realtimeOpenAIModels() = %v, want %v", got, want)
	}
}

func TestFetchGoogleModelsIntegration(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY не задан в окружении")
	}

	models, err := NewClient().fetchGoogleModels(context.Background(), apiKey)
	if err != nil {
		t.Fatalf("fetchGoogleModels() returned error: %v", err)
	}

	fmt.Println("ALL models:", models)

	if len(models) == 0 {
		t.Fatal("fetchGoogleModels() returned no models")
	}
}

func TestGeneralGoogleModelsIntegration(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY не задан в окружении")
	}

	models, err := NewClient().generalGoogleModels(context.Background(), apiKey)
	if err != nil {
		t.Fatalf("generalGoogleModels() returned error: %v", err)
	}

	fmt.Println(models)

	if len(models) == 0 {
		t.Fatal("generalGoogleModels() returned no models")
	}
	for _, model := range models {
		if !isGeneralGoogleModel(model) {
			t.Errorf("generalGoogleModels() returned specialized model %q", model)
		}
	}
}

func TestFetchMistralRealtimeModelsIntegration(t *testing.T) {
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		t.Skip("MISTRAL_API_KEY не задан в окружении")
	}

	models, err := NewClient().realtimeMistralModels(context.Background(), apiKey)
	if err != nil {
		t.Fatalf("realtimeMistralModels() returned error: %v", err)
	}

	fmt.Println(models)

	if len(models) == 0 {
		t.Fatal("realtimeMistralModels() returned no models")
	}
	for _, model := range models {
		if !isRealtimeModel(model) {
			t.Errorf("realtimeMistralModels() returned specialized model %q", model)
		}
	}
}

func TestGeneralMistralModelsIntegration(t *testing.T) {
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		t.Skip("MISTRAL_API_KEY не задан в окружении")
	}

	models, err := NewClient().generalMistralModels(context.Background(), apiKey)
	if err != nil {
		t.Fatalf("generalMistralModels() returned error: %v", err)
	}

	fmt.Println("mistral general:", models)

	if len(models) == 0 {
		t.Fatal("generalMistralModels() returned no models")
	}
	for _, model := range models {
		if !isGeneralMistralModel(model) {
			t.Errorf("generalMistralModels() returned specialized model %q", model)
		}
	}
}

func TestMistralAllModelsIntegration(t *testing.T) {
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		t.Skip("MISTRAL_API_KEY не задан в окружении")
	}

	models, err := NewClient().fetchMistralModels(context.Background(), apiKey)
	if err != nil {
		t.Fatalf("generalMistralModels() returned error: %v", err)
	}

	fmt.Println("all mistral general:", models)

	if len(models) == 0 {
		t.Fatal("generalMistralModels() returned no models")
	}
}

func TestIsGeneralGoogleModelKeepsChatModelsOnly(t *testing.T) {
	keep := []string{
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-2.5-pro",
		"gemini-3-flash-preview",
		"gemini-3.1-flash-lite",
		"gemini-3.1-pro-preview",
		"gemini-3.8-flash",
		"gemini-flash-latest",
	}
	for _, name := range keep {
		if !isGeneralGoogleModel(name) {
			t.Errorf("isGeneralGoogleModel(%q) = false, want true", name)
		}
	}

	drop := []string{
		"gemini-3-pro-image",
		"gemini-3.1-flash-image-preview",
		"gemini-2.5-flash-live-preview",
		"gemini-omni-1.1-flash",
		"gemini-omni-flash-preview",
		"gemini-2.5-computer-use-preview-10-2025",
		"gemini-robotics-er-2-preview",
		"gemini-nano-banana-2.1",
		"gemma-4-31b-it",
		"lyria-3-pro-preview",
		"deep-research-pro-preview-12-2025",
		"antigravity-preview-latest",
	}
	for _, name := range drop {
		if isGeneralGoogleModel(name) {
			t.Errorf("isGeneralGoogleModel(%q) = true, want false", name)
		}
	}
}

func TestRealtimeGoogleModelsFiltersBidiGenerateContent(t *testing.T) {
	client := &Client{
		GoogleBaseURL: "https://google.test/v1beta",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.URL.Query().Get("key"); got != "test-key" {
					t.Errorf("key = %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"models": [
							{"name":"models/gemini-3.1-flash-live-preview","supportedGenerationMethods":["generateContent","bidiGenerateContent"]},
							{"name":"models/gemini-2.5-flash-live-preview","supportedGenerationMethods":["generateContent"]},
							{"name":"models/gemini-omni-flash-preview","supportedGenerationMethods":["generateContent","embedContent"]},
							{"name":"models/gemini-2.0-flash-exp","supportedGenerationMethods":["generateContent","bidiGenerateContent"]}
						]
					}`)),
				}, nil
			}),
		},
	}

	got, err := client.realtimeGoogleModels(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("realtimeGoogleModels() error: %v", err)
	}
	want := []string{"gemini-3.1-flash-live-preview"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("realtimeGoogleModels() = %v, want %v", got, want)
	}
}

func TestRealtimeGoogleModelsKeepsOnlyLiveNames(t *testing.T) {
	client := &Client{
		GoogleBaseURL: "https://google.test/v1beta",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"models": [
							{"name":"models/gemini-3.1-flash-live-preview"},
							{"name":"models/gemini-2.5-flash"},
							{"name":"models/gemini-2.0-flash-exp"},
							{"name":"models/gemini-live-2.5-flash-preview"}
						]
					}`)),
				}, nil
			}),
		},
	}

	got, err := client.realtimeGoogleModels(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("realtimeGoogleModels() error: %v", err)
	}
	want := []string{"gemini-3.1-flash-live-preview", "gemini-live-2.5-flash-preview"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("realtimeGoogleModels() = %v, want %v", got, want)
	}
}

func TestRealtimeGoogleModelsPaginates(t *testing.T) {
	client := &Client{
		GoogleBaseURL: "https://google.test/v1beta",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := `{
					"models": [
						{"name":"models/gemini-3.1-flash-live-preview","supportedGenerationMethods":["bidiGenerateContent"]}
					],
					"nextPageToken":"page-2"
				}`
				if req.URL.Query().Get("pageToken") == "page-2" {
					body = `{
						"models": [
							{"name":"models/gemini-omni-flash-preview","supportedGenerationMethods":["generateContent"]},
							{"name":"models/gemini-live-2.5-flash-preview","supportedGenerationMethods":["bidiGenerateContent"]}
						]
					}`
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
	}

	got, err := client.realtimeGoogleModels(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("realtimeGoogleModels() error: %v", err)
	}
	want := []string{"gemini-3.1-flash-live-preview", "gemini-live-2.5-flash-preview"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("realtimeGoogleModels() = %v, want %v", got, want)
	}
}

func TestMistralSpecializedModelFilters(t *testing.T) {
	models := []string{
		"voxtral-mini-transcribe-realtime-2602",
		"voxtral-mini-realtime-latest",
		"voxtral-mini-tts-2603",
		"voxtral-mini-tts-latest",
		"mistral-small-latest",
	}
	if got := filterMistralModels(models, isMistralSTTModel); !reflect.DeepEqual(got, []string{"voxtral-mini-transcribe-realtime-2602"}) {
		t.Fatalf("STT filter = %v", got)
	}
	if got := filterMistralModels(models, isMistralTTSModel); !reflect.DeepEqual(got, []string{"voxtral-mini-tts-2603", "voxtral-mini-tts-latest"}) {
		t.Fatalf("TTS filter = %v", got)
	}
}
