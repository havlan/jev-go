package jev

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

	hasNoul          bool
	hasChoice        bool
	hasScore         bool
	hasLegend        bool
	hasProbabilities bool
	hasConfidence    bool
}

func (a *Answer) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type          QuestionType       `json:"type"`
		Noul          *float64           `json:"noul"`
		Choice        *string            `json:"choice"`
		Score         *float64           `json:"score"`
		Legend        map[string]string  `json:"legend"`
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    *float64           `json:"confidence"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Type = raw.Type
	a.Legend = raw.Legend
	a.Probabilities = raw.Probabilities
	a.hasNoul = raw.Noul != nil
	a.hasChoice = raw.Choice != nil
	a.hasScore = raw.Score != nil
	a.hasLegend = raw.Legend != nil
	a.hasProbabilities = raw.Probabilities != nil
	a.hasConfidence = raw.Confidence != nil
	if raw.Noul != nil {
		a.Noul = *raw.Noul
	}
	if raw.Choice != nil {
		a.Choice = *raw.Choice
	}
	if raw.Score != nil {
		a.Score = *raw.Score
	}
	if raw.Confidence != nil {
		a.Confidence = *raw.Confidence
	}
	return nil
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

func validateResponse(response Response, questions Questions) error {
	if response.Model == "" {
		return errors.New("Jev response is missing model")
	}
	for id, question := range questions {
		answer, ok := response.Answers[id]
		if !ok {
			return fmt.Errorf("Jev response is missing answer %q", id)
		}
		if answer.Type != question.questionType() {
			return fmt.Errorf("answer %q has type %q, want %q", id, answer.Type, question.questionType())
		}
	}
	for id, question := range questions {
		if err := validateAnswer(id, response.Answers[id], question); err != nil {
			return err
		}
	}
	return nil
}

func validateAnswer(id string, answer Answer, question Question) error {
	switch answer.Type {
	case QuestionNoul:
		if !answer.hasNoul {
			return fmt.Errorf("answer %q is missing noul", id)
		}
		if answer.Noul < 0 || answer.Noul > 1 {
			return fmt.Errorf("answer %q has invalid noul value %v", id, answer.Noul)
		}
		return nil
	case QuestionChoice:
		choice, ok := question.(choiceQuestion)
		if !ok {
			return fmt.Errorf("answer %q has invalid choice question", id)
		}
		if !answer.hasChoice || answer.Choice == "" {
			return fmt.Errorf("answer %q has an empty choice", id)
		}
		if _, ok := choice.Criteria[answer.Choice]; !ok {
			return fmt.Errorf("answer %q chose unknown criterion %q", id, answer.Choice)
		}
		if !answer.hasProbabilities {
			return fmt.Errorf("answer %q is missing probabilities", id)
		}
		if err := validateProbabilities(id, answer.Probabilities); err != nil {
			return err
		}
		if err := validateProbabilityKeys(id, answer.Probabilities, choice.Criteria); err != nil {
			return err
		}
		if !answer.hasConfidence {
			return fmt.Errorf("answer %q is missing confidence", id)
		}
		return validateConfidence(id, answer.Confidence)
	case QuestionScore:
		if !answer.hasScore {
			return fmt.Errorf("answer %q is missing score", id)
		}
		if !answer.hasLegend {
			return fmt.Errorf("answer %q is missing legend", id)
		}
		if !answer.hasProbabilities {
			return fmt.Errorf("answer %q is missing probabilities", id)
		}
		if len(answer.Legend) < 2 || answer.Score < 0 || answer.Score > float64(len(answer.Legend)-1) {
			return fmt.Errorf("answer %q has invalid score %v", id, answer.Score)
		}
		if err := validateProbabilities(id, answer.Probabilities); err != nil {
			return err
		}
		if err := validateScoreKeys(id, answer.Legend, answer.Probabilities); err != nil {
			return err
		}
		if !answer.hasConfidence {
			return fmt.Errorf("answer %q is missing confidence", id)
		}
		return validateConfidence(id, answer.Confidence)
	default:
		return fmt.Errorf("answer %q has unknown type %q", id, answer.Type)
	}
}

func validateProbabilities(id string, probabilities map[string]float64) error {
	if len(probabilities) == 0 {
		return fmt.Errorf("answer %q has empty probabilities", id)
	}
	var total float64
	for option, probability := range probabilities {
		if probability < 0 || probability > 1 {
			return fmt.Errorf("answer %q has invalid probability for %q: %v", id, option, probability)
		}
		total += probability
	}
	if total < 0.999 || total > 1.001 {
		return fmt.Errorf("answer %q probabilities sum to %v, want 1", id, total)
	}
	return nil
}

func validateProbabilityKeys(id string, probabilities map[string]float64, criteria ChoiceCriteria) error {
	if len(probabilities) != len(criteria) {
		return fmt.Errorf("answer %q probabilities do not match choice criteria", id)
	}
	for option := range criteria {
		if _, ok := probabilities[option]; !ok {
			return fmt.Errorf("answer %q probabilities are missing criterion %q", id, option)
		}
	}
	return nil
}

func validateScoreKeys(id string, legend map[string]string, probabilities map[string]float64) error {
	if len(legend) != len(probabilities) {
		return fmt.Errorf("answer %q legend and probabilities do not match", id)
	}
	for level := range legend {
		if _, ok := probabilities[level]; !ok {
			return fmt.Errorf("answer %q probabilities are missing score level %q", id, level)
		}
	}
	for level := 0; level < len(legend); level++ {
		key := fmt.Sprint(level)
		if _, ok := legend[key]; !ok {
			return fmt.Errorf("answer %q legend is missing score level %q", id, key)
		}
	}
	return nil
}

func validateConfidence(id string, confidence float64) error {
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("answer %q has invalid confidence %v", id, confidence)
	}
	return nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	kind := reflect.ValueOf(value).Kind()
	return (kind == reflect.Chan || kind == reflect.Func || kind == reflect.Interface || kind == reflect.Map || kind == reflect.Pointer || kind == reflect.Slice) && reflect.ValueOf(value).IsNil()
}

func validateJSONValue(value any, name string) error {
	if isNil(value) {
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
