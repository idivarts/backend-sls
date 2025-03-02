package instagram

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/idivarts/backend-sls/pkg/messenger"
)

type InsightParams struct {
	Timeframe  string
	MetricType string
	Breakdown  string
	StartTime  string
	StopTime   string
}

// joinWithComma joins a slice of strings with commas
func joinWithComma(items []string) string {
	return strings.Join(items, ",")
}

func GetInsights(
	pageID,
	accessToken string,
	metrics []string,
	period string,
	params InsightParams,
) (*messenger.InstagramBriefProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/insights", BaseURL, ApiVersion, pageID)
	// Create query parameters
	iParam := url.Values{}
	iParam.Set("metric", joinWithComma(metrics))
	iParam.Set("period", period)
	if params.Timeframe != "" {
		iParam.Set("timeframe", params.Timeframe)
	}
	if params.MetricType != "" {
		iParam.Set("metric_type", params.MetricType)
	}
	if params.Breakdown != "" {
		iParam.Set("breakdown", params.Breakdown)
	}
	if params.StartTime != "" {
		iParam.Set("since", params.StartTime)
	}
	if params.StopTime != "" {
		iParam.Set("until", params.StopTime)
	}
	iParam.Set("access_token", accessToken)

	allParams := iParam.Encode()
	log.Println("All Params:", allParams)

	// Combine base URL and query parameters
	apiURL = fmt.Sprintf("%s?%s", apiURL, allParams)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	// Print the response body
	fmt.Println(string(body))
	// data := messenger.InstagramBriefProfile{}
	// err = json.Unmarshal(body, &data)
	// if err != nil {
	// 	return nil, err
	// }
	return nil, nil
}
