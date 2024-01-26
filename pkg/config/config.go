package config

type DatadogCredential struct {
	APIKey string `json:"apiKey"`
	AppKey string `json:"appKey"`
}

type Config struct {
	Credentials map[string]DatadogCredential `json:"credentials"`
}
