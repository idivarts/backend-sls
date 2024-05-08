package openaifc

import (
	"encoding/json"
	"reflect"
)

type ChangePhase struct {
	Phase                int    `json:"phase" dynamodbav:"phase"`
	Engagement           string `json:"engagement" dynamodbav:"engagement"`
	Views                string `json:"views" dynamodbav:"views"`
	VideoCategory        string `json:"video_category" dynamodbav:"video_category"`
	BrandCategory        string `json:"brand_category" dynamodbav:"brand_category"`
	InterestedInService  *bool  `json:"interestInService,omitempty" dynamodbav:"interestInService"`
	InterestedInApp      *bool  `json:"interestInApp,omitempty" dynamodbav:"interestInApp"`
	CollaborationBrand   string `json:"collaboration_brand" dynamodbav:"collaboration_brand"`
	CollaborationProduct string `json:"collaboration_product" dynamodbav:"collaboration_product"`
}

func (c *ChangePhase) ParseJson(str string) error {
	err := json.Unmarshal([]byte(str), c)
	if err != nil {
		return err
	}
	return nil
}

func (input ChangePhase) FindEmptyFields() ([]string, error) {
	var emptyFields []string

	val := reflect.ValueOf(input)
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		jsonTag := val.Type().Field(i).Tag.Get("json")
		if field.Kind() == reflect.String {
			if field.String() == "" {
				emptyFields = append(emptyFields, jsonTag)
			}
		} else if field.Kind() == reflect.Ptr && field.IsNil() {
			emptyFields = append(emptyFields, jsonTag)
		}
	}
	// b, err := json.Marshal(emptyFields)
	// if err != nil {
	// 	return nil, err
	// }
	// x := string(b)
	return emptyFields, nil
}
