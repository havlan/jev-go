package jev

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSystemOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var request struct {
			Model     string                     `json:"model"`
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != DefaultModel || len(request.Questions) != 3 {
			t.Fatalf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"jev-latest",
			"answers":{
				"urgent":{"type":"noul","noul":0.99},
				"department":{"type":"choice","choice":"technical","probabilities":{"technical":0.9,"billing":0.1},"confidence":0.8},
				"frustration":{"type":"score","score":1.4,"legend":{"0":"calm","1":"frustrated","2":"angry"},"probabilities":{"0":0.1,"1":0.4,"2":0.5},"confidence":0.7}
			},
			"usage":{"input_tokens":10,"output_tokens":5}
		}`))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: " test-key ", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.SystemOne(context.Background(), "help ASAP", Questions{
		"urgent":      Noul("Is this urgent?"),
		"department":  Choice("Which team?", StringCriteria(map[string]string{"billing": "payments", "technical": "bugs"})),
		"frustration": Score("How frustrated?", "calm", "frustrated", "angry"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Answers["urgent"].Noul != 0.99 || response.Answers["department"].Choice != "technical" || response.Answers["frustration"].Score != 1.4 {
		t.Fatalf("response = %+v", response)
	}
	if _, err := response.Answer("urgent", QuestionNoul); err != nil {
		t.Fatal(err)
	}
}

func TestRetriesOverload(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(529)
			return
		}
		_, _ = w.Write([]byte(`{"model":"jev-latest","answers":{"ok":{"type":"noul","noul":1}},"usage":{}}`))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SystemOne(context.Background(), "state", Questions{"ok": Noul("Is this okay?")}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"invalid key"}`))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "bad-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SystemOne(context.Background(), "state", Questions{"ok": Noul("Is this okay?")})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized || !strings.Contains(apiErr.Body, "invalid key") {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsInvalidQuestion(t *testing.T) {
	client, err := New(Config{APIKey: "test-key", MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SystemOne(context.Background(), "state", Questions{"score": Score("Rate it", "only one")})
	if err == nil || !strings.Contains(err.Error(), "between two and ten") {
		t.Fatalf("error = %v", err)
	}
}

func TestQuestionAndResponseValidation(t *testing.T) {
	if err := validateRequest(Request{
		State: 42,
		Questions: Questions{
			"ok": Noul("Is this okay?"),
		},
	}); err == nil {
		t.Fatal("numeric state should be rejected")
	}

	choice := Choice("Which?", ChoiceCriteria{"billing": nil, "technical": nil})
	encoded, err := json.Marshal(choice)
	if err != nil || !strings.Contains(string(encoded), `"billing":null`) {
		t.Fatalf("choice JSON = %s, error = %v", encoded, err)
	}

	var missingNoul Response
	if err := json.Unmarshal([]byte(`{"model":"jev-latest","answers":{"ok":{"type":"noul"}}}`), &missingNoul); err != nil {
		t.Fatal(err)
	}
	if err := validateResponse(missingNoul, Questions{"ok": Noul("Is this okay?")}); err == nil {
		t.Fatal("missing noul should be rejected")
	}

	var missingChoiceProbabilities Response
	if err := json.Unmarshal([]byte(`{"model":"jev-latest","answers":{"kind":{"type":"choice","choice":"billing","confidence":0.9}}}`), &missingChoiceProbabilities); err != nil {
		t.Fatal(err)
	}
	if err := validateResponse(missingChoiceProbabilities, Questions{
		"kind": Choice("Which?", ChoiceCriteria{"billing": nil, "technical": nil}),
	}); err == nil {
		t.Fatal("missing choice probabilities should be rejected")
	}

	var missingScoreFields Response
	if err := json.Unmarshal([]byte(`{"model":"jev-latest","answers":{"severity":{"type":"score","score":1,"confidence":0.9}}}`), &missingScoreFields); err != nil {
		t.Fatal(err)
	}
	if err := validateResponse(missingScoreFields, Questions{
		"severity": Score("How severe?", "low", "high"),
	}); err == nil {
		t.Fatal("missing score fields should be rejected")
	}
}

func TestRejectsEmptyInstructions(t *testing.T) {
	for name, question := range Questions{
		"noul":   Noul(" "),
		"choice": Choice("", ChoiceCriteria{"yes": nil, "no": nil}),
		"score":  Score("\t", "low", "high"),
	} {
		t.Run(name, func(t *testing.T) {
			err := validateRequest(Request{State: "state", Questions: Questions{name: question}})
			if err == nil || !strings.Contains(err.Error(), "instructions must not be empty") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRejectsMissingChoiceConfidence(t *testing.T) {
	var response Response
	if err := json.Unmarshal([]byte(`{
		"model":"jev-latest",
		"answers":{
			"department":{
				"type":"choice",
				"choice":"technical",
				"probabilities":{"technical":0.9,"billing":0.1}
			}
		}
	}`), &response); err != nil {
		t.Fatal(err)
	}

	err := validateResponse(response, Questions{
		"department": Choice("Which team?", ChoiceCriteria{"billing": nil, "technical": nil}),
	})
	if err == nil || !strings.Contains(err.Error(), "missing confidence") {
		t.Fatalf("error = %v", err)
	}
}
