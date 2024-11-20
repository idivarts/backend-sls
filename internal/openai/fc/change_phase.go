package openaifc

import (
	"encoding/json"
	"reflect"
)

type ChangePhase struct {
	Phase                int    `json:"phase" firestore:"phase"`
	Engagement           string `json:"engagement" firestore:"engagement"`
	EngagementUnit       string `json:"engagement_unit" firestore:"engagement_unit"`
	Views                string `json:"views" firestore:"views"`
	ViewsUnit            string `json:"views_unit" firestore:"views_unit"`
	VideoCategory        string `json:"video_category" firestore:"video_category"`
	BrandCategory        string `json:"brand_category" firestore:"brand_category"`
	InterestedInService  *bool  `json:"interestInService,omitempty" firestore:"interestInService"`
	InterestedInApp      *bool  `json:"interestInApp,omitempty" firestore:"interestInApp"`
	CollaborationBrand   string `json:"collaboration_brand" firestore:"collaboration_brand"`
	CollaborationProduct string `json:"collaboration_product" firestore:"collaboration_product"`
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
