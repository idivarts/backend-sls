package openaifc

import (
	"encoding/json"
	"reflect"
)

type CanConversationEnd struct {
	Engagement    string `json:"engagement"`
	Views         string `json:"views"`
	VideoCategory string `json:"video_category"`
	BrandCategory string `json:"brand_category"`
}

func (c *CanConversationEnd) ParseJson(str string) error {
	err := json.Unmarshal([]byte(str), c)
	if err != nil {
		return err
	}
	return nil
}

func (input CanConversationEnd) FindEmptyFields() (*string, error) {
	var emptyFields []string

	val := reflect.ValueOf(input)
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if field.Kind() == reflect.String && field.String() == "" {
			emptyFields = append(emptyFields, val.Type().Field(i).Tag.Get("json"))
		}
	}
	b, err := json.Marshal(emptyFields)
	if err != nil {
		return nil, err
	}
	x := string(b)
	return &x, nil
}
