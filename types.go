package jev

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type QuestionType string

const (
	QuestionNoul   QuestionType = "noul"
	QuestionChoice QuestionType = "choice"
	QuestionScore  QuestionType = "score"
)

type Question interface {
	json.Marshaler
	questionType() QuestionType
	validate() error
}

type Questions map[string]Question

type NoulCriteria struct {
	True  string `json:"true,omitempty"`
	False string `json:"false,omitempty"`
}

type ChoiceCriteria map[string]*string

func StringCriteria(criteria map[string]string) ChoiceCriteria {
	result := make(ChoiceCriteria, len(criteria))
	for option, description := range criteria {
		descriptionCopy := description
		result[option] = &descriptionCopy
	}
	return result
}

type noulQuestion struct {
	Instructions any
	Criteria     []NoulCriteria
}

func Noul(instructions any, criteria ...NoulCriteria) Question {
	return noulQuestion{Instructions: instructions, Criteria: criteria}
}

func (q noulQuestion) MarshalJSON() ([]byte, error) {
	var criteria *NoulCriteria
	if len(q.Criteria) == 1 {
		criteria = &q.Criteria[0]
	}
	return json.Marshal(struct {
		Type         QuestionType  `json:"type"`
		Instructions any           `json:"instructions"`
		Criteria     *NoulCriteria `json:"criteria,omitempty"`
	}{QuestionNoul, q.Instructions, criteria})
}

func (q noulQuestion) questionType() QuestionType { return QuestionNoul }

func (q noulQuestion) validate() error {
	if len(q.Criteria) > 1 {
		return errors.New("noul accepts at most one criteria value")
	}
	return validateInstructions(q.Instructions)
}

type choiceQuestion struct {
	Instructions any
	Criteria     ChoiceCriteria
}

func Choice(instructions any, criteria ChoiceCriteria) Question {
	return choiceQuestion{Instructions: instructions, Criteria: criteria}
}

func (q choiceQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         QuestionType   `json:"type"`
		Instructions any            `json:"instructions"`
		Criteria     ChoiceCriteria `json:"criteria"`
	}{QuestionChoice, q.Instructions, q.Criteria})
}

func (q choiceQuestion) questionType() QuestionType { return QuestionChoice }

func (q choiceQuestion) validate() error {
	if err := validateInstructions(q.Instructions); err != nil {
		return err
	}
	if len(q.Criteria) < 2 {
		return errors.New("choice requires at least two criteria")
	}
	for option := range q.Criteria {
		if strings.TrimSpace(option) == "" {
			return errors.New("choice criterion names must not be empty")
		}
	}
	return nil
}

type scoreQuestion struct {
	Instructions any
	Criteria     []string
}

func Score(instructions any, criteria ...string) Question {
	return scoreQuestion{Instructions: instructions, Criteria: criteria}
}

func (q scoreQuestion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         QuestionType `json:"type"`
		Instructions any          `json:"instructions"`
		Criteria     []string     `json:"criteria"`
	}{QuestionScore, q.Instructions, q.Criteria})
}

func (q scoreQuestion) questionType() QuestionType { return QuestionScore }

func (q scoreQuestion) validate() error {
	if err := validateInstructions(q.Instructions); err != nil {
		return err
	}
	if len(q.Criteria) < 2 || len(q.Criteria) > 10 {
		return errors.New("score requires between two and ten criteria")
	}
	for _, level := range q.Criteria {
		if strings.TrimSpace(level) == "" {
			return errors.New("score criteria must not be empty")
		}
	}
	return nil
}

func validateInstructions(instructions any) error {
	if err := validateJSONValue(instructions, "instructions"); err != nil {
		return err
	}
	if text, ok := instructions.(string); ok && strings.TrimSpace(text) == "" {
		return errors.New("instructions must not be empty")
	}
	return nil
}

type Request struct {
	State     any       `json:"state"`
	Model     string    `json:"model,omitempty"`
	Questions Questions `json:"questions"`
}

type Answer struct {
	Type          QuestionType       `json:"type"`
	Noul          float64            `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         float64            `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

func (r Response) Answer(id string, expected QuestionType) (Answer, error) {
	answer, ok := r.Answers[id]
	if !ok {
		return Answer{}, fmt.Errorf("answer %q is missing", id)
	}
	if answer.Type != expected {
		return Answer{}, fmt.Errorf("answer %q has type %q, want %q", id, answer.Type, expected)
	}
	return answer, nil
}

func validateRequest(request Request) error {
	if err := validateJSONValue(request.State, "state"); err != nil {
		return err
	}
	if len(request.Questions) == 0 {
		return errors.New("at least one question is required")
	}
	for id, question := range request.Questions {
		if strings.TrimSpace(id) == "" {
			return errors.New("question IDs must not be empty")
		}
		if question == nil {
			return fmt.Errorf("question %q is nil", id)
		}
		if err := question.validate(); err != nil {
			return fmt.Errorf("question %q: %w", id, err)
		}
	}
	return nil
}

func validateJSONValue(value any, name string) error {
	if value == nil {
		return fmt.Errorf("%s must not be nil", name)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%s must be JSON-serializable: %w", name, err)
	}
	trimmed := strings.TrimSpace(string(encoded))
	if trimmed == "" || trimmed == "null" {
		return fmt.Errorf("%s must not be null", name)
	}
	switch trimmed[0] {
	case '"', '{', '[':
		return nil
	default:
		return fmt.Errorf("%s must be a string, object, or array", name)
	}
}
