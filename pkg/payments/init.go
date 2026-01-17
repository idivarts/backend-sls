package payments

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/idivarts/backend-sls/pkg/myutil"
	razorpay "github.com/razorpay/razorpay-go"
)

var (
	Client     *razorpay.Client
	apiKey     = ""
	apiSecret  = ""
	WebhookKey = ""
	Plans      = map[string]string{}
)

const (
	RedirectUrl = "https://brands.trendly.now"
)

type RazorpaySecrets struct {
	APIKey     string            `json:"key"`
	APISecret  string            `json:"secret"`
	WebhookKey string            `json:"webhookKey"`
	Plans      map[string]string `json:"plans"`
}

type KeySecretJson struct {
	RazorPay RazorpaySecrets `json:"razorpay"`
}

func loadSecrets() RazorpaySecrets {
	basePath := "."
	if myutil.IsTest() {
		basePath = "/Users/rsinha/iDiv/backend-sls/"
	}
	path := filepath.Join(basePath, "key-secrets.json")
	file, err := os.Open(path)
	if err != nil {
		log.Printf("could not open key-secrets.json: %v", err)
		return RazorpaySecrets{}
	}
	defer file.Close()

	var secrets KeySecretJson
	if err := json.NewDecoder(file).Decode(&secrets); err != nil {
		log.Printf("could not decode key-secrets.json: %v", err)
		return RazorpaySecrets{}
	}

	return secrets.RazorPay
}

func init() {
	secrets := loadSecrets()
	apiKey = secrets.APIKey
	apiSecret = secrets.APISecret
	WebhookKey = secrets.WebhookKey
	Plans = secrets.Plans

	if apiKey == "" || apiSecret == "" {
		apiKey = "rzp_test_Z9T0fM1E1agkpR"
		apiSecret = "LaqAVYPBdqdrC4psaoga18nE"
		WebhookKey = "rzp_test_webhook_1234567890"
	}

	Client = razorpay.NewClient(apiKey, apiSecret)
}
